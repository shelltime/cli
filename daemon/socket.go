package daemon

import (
	"encoding/json"
	"log/slog"
	"net"
	"os"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/malamtime/cli/model"
)

type SocketMessageType string

const (
	SocketMessageTypeSync SocketMessageType = "sync"
)

type SocketMessage struct {
	Type SocketMessageType `json:"type"`
	// if parse from buffer, it will be the map[any]any
	Payload interface{} `json:"payload"`
}

type SocketHandler struct {
	config   *model.ShellTimeConfig
	listener net.Listener

	channel  *GoChannel
	stopChan chan struct{}
}

func NewSocketHandler(config *model.ShellTimeConfig, ch *GoChannel) *SocketHandler {
	return &SocketHandler{
		config:   config,
		channel:  ch,
		stopChan: make(chan struct{}),
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
	// case "status":
	// 	p.handleStatus(conn)
	// case "track":
	// 	p.handleTrack(conn, msg.Payload)
	case SocketMessageTypeSync:
		buf, err := json.Marshal(msg)
		if err != nil {
			slog.Error("Error encoding message", slog.Any("err", err))
		}

		chMsg := message.NewMessage(watermill.NewUUID(), buf)
		if err := p.channel.Publish(PubSubTopic, chMsg); err != nil {
			slog.Error("Error to publish topic", slog.Any("err", err))
		}
	default:
		slog.Error("Unknown message type:", slog.String("messageType", string(msg.Type)))
	}
}
