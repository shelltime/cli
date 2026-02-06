package daemon

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
)

type SocketMessageType string

const (
	SocketMessageTypeSync      SocketMessageType = "sync"
	SocketMessageTypeHeartbeat SocketMessageType = "heartbeat"
	SocketMessageTypeStatus    SocketMessageType = "status"
	SocketMessageTypeCCInfo    SocketMessageType = "cc_info"
)

type CCInfoTimeRange string

const (
	CCInfoTimeRangeToday CCInfoTimeRange = "today"
	CCInfoTimeRangeWeek  CCInfoTimeRange = "week"
	CCInfoTimeRangeMonth CCInfoTimeRange = "month"
)

type CCInfoRequest struct {
	TimeRange        CCInfoTimeRange `json:"timeRange"`
	WorkingDirectory string          `json:"workingDirectory"`
}

type CCInfoResponse struct {
	TotalCostUSD        float64   `json:"totalCostUsd"`
	TotalSessionSeconds int       `json:"totalSessionSeconds"`
	TimeRange           string    `json:"timeRange"`
	CachedAt            time.Time `json:"cachedAt"`
	GitBranch           string    `json:"gitBranch"`
	GitDirty            bool      `json:"gitDirty"`
	FiveHourUtilization *float64  `json:"fiveHourUtilization,omitempty"`
	SevenDayUtilization *float64  `json:"sevenDayUtilization,omitempty"`
	UserLogin           string    `json:"userLogin,omitempty"`
}

// StatusResponse contains daemon status information
type StatusResponse struct {
	Version   string    `json:"version"`
	StartedAt time.Time `json:"startedAt"`
	Uptime    string    `json:"uptime"`
	GoVersion string    `json:"goVersion"`
	Platform  string    `json:"platform"`
}

type SocketMessage struct {
	Type SocketMessageType `json:"type"`
	// if parse from buffer, it will be the map[any]any
	Payload interface{} `json:"payload"`
}

type SocketHandler struct {
	config   *model.ShellTimeConfig
	listener net.Listener

	channel     *GoChannel
	stopChan    chan struct{}
	ccInfoTimer *CCInfoTimerService
}

func NewSocketHandler(config *model.ShellTimeConfig, ch *GoChannel) *SocketHandler {
	return &SocketHandler{
		config:      config,
		channel:     ch,
		stopChan:    make(chan struct{}),
		ccInfoTimer: NewCCInfoTimerService(config),
	}
}

func (p *SocketHandler) Start() error {
	// Remove existing socket file if it exists
	if err := os.RemoveAll(p.config.SocketPath); err != nil {
		return err
	}

	// Create Unix domain socket
	listener, err := net.Listen("unix", p.config.SocketPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(p.config.SocketPath, 0777); err != nil {
		slog.Error("Failed to change the socket permission to 0755", slog.String("socketPath", p.config.SocketPath))
		return err
	}
	p.listener = listener

	// Start accepting connections
	go p.acceptConnections()

	slog.Info("Daemon started, listening on: ", slog.String("socketPath", p.config.SocketPath))
	return nil
}

func (p *SocketHandler) Stop() {
	p.channel.Close()
	close(p.stopChan)
	if p.ccInfoTimer != nil {
		p.ccInfoTimer.Stop()
	}
	if p.listener != nil {
		p.listener.Close()
	}
	os.RemoveAll(p.config.SocketPath)
	slog.Info("Daemon stopped")
}

func (p *SocketHandler) acceptConnections() {
	for {
		select {
		case <-p.stopChan:
			return
		default:
			conn, err := p.listener.Accept()
			if err != nil {
				continue
			}
			go p.handleConnection(conn)
		}
	}
}

func (p *SocketHandler) handleConnection(conn net.Conn) {
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	var msg SocketMessage
	if err := decoder.Decode(&msg); err != nil {
		slog.Error("Error decoding message", slog.Any("err", err))
		return
	}

	switch msg.Type {
	case SocketMessageTypeStatus:
		p.handleStatus(conn)
	case SocketMessageTypeSync:
		buf, err := json.Marshal(msg)
		if err != nil {
			slog.Error("Error encoding message", slog.Any("err", err))
		}

		chMsg := message.NewMessage(watermill.NewUUID(), buf)
		if err := p.channel.Publish(PubSubTopic, chMsg); err != nil {
			slog.Error("Error to publish topic", slog.Any("err", err))
		}
	case SocketMessageTypeHeartbeat:
		// Only process heartbeat if codeTracking is enabled
		if p.config.CodeTracking == nil || p.config.CodeTracking.Enabled == nil || !*p.config.CodeTracking.Enabled {
			slog.Debug("Heartbeat message received but codeTracking is disabled, ignoring")
			encoder := json.NewEncoder(conn)
			encoder.Encode(map[string]string{"status": "disabled"})
			return
		}
		buf, err := json.Marshal(msg)
		if err != nil {
			slog.Error("Error encoding heartbeat message", slog.Any("err", err))
			return
		}

		chMsg := message.NewMessage(watermill.NewUUID(), buf)
		if err := p.channel.Publish(PubSubTopic, chMsg); err != nil {
			slog.Error("Error publishing heartbeat topic", slog.Any("err", err))
		}

		// Send acknowledgment to client
		encoder := json.NewEncoder(conn)
		encoder.Encode(map[string]string{"status": "ok"})
	case SocketMessageTypeCCInfo:
		p.handleCCInfo(conn, msg)
	default:
		slog.Error("Unknown message type:", slog.String("messageType", string(msg.Type)))
	}
}

func (p *SocketHandler) handleStatus(conn net.Conn) {
	uptime := time.Since(startedAt)
	response := StatusResponse{
		Version:   version,
		StartedAt: startedAt,
		Uptime:    formatDuration(uptime),
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(response); err != nil {
		slog.Error("Error encoding status response", slog.Any("err", err))
	}
}

func (p *SocketHandler) handleCCInfo(conn net.Conn, msg SocketMessage) {
	slog.Debug("cc_info socket event received")

	// Parse time range and working directory from payload
	timeRange := CCInfoTimeRangeToday
	var workingDir string
	if payload, ok := msg.Payload.(map[string]interface{}); ok {
		if tr, ok := payload["timeRange"].(string); ok {
			timeRange = CCInfoTimeRange(tr)
		}
		if wd, ok := payload["workingDirectory"].(string); ok {
			workingDir = wd
		}
	}

	// Get cached cost first (marks range as active), then notify activity (starts timer)
	cache := p.ccInfoTimer.GetCachedCost(timeRange)
	p.ccInfoTimer.NotifyActivity()

	// Get git info (cached to avoid slow worktree.Status() on large repos)
	gitInfo := p.ccInfoTimer.GetCachedGitInfo(workingDir)

	response := CCInfoResponse{
		TotalCostUSD:        cache.TotalCostUSD,
		TotalSessionSeconds: cache.TotalSessionSeconds,
		TimeRange:           string(timeRange),
		CachedAt:            cache.FetchedAt,
		GitBranch:           gitInfo.Branch,
		GitDirty:            gitInfo.Dirty,
		UserLogin:           p.ccInfoTimer.GetCachedUserLogin(),
	}

	// Populate rate limit fields if available
	if rl := p.ccInfoTimer.GetCachedRateLimit(); rl != nil {
		response.FiveHourUtilization = &rl.FiveHourUtilization
		response.SevenDayUtilization = &rl.SevenDayUtilization
	}

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(response); err != nil {
		slog.Error("Error encoding cc_info response", slog.Any("err", err))
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
