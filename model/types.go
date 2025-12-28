package model

const (
	DefaultSocketPath = "/tmp/shelltime.sock"
)

type Endpoint struct {
	APIEndpoint string `toml:"APIEndpoint" yaml:"apiEndpoint" json:"apiEndpoint"`
	Token       string `toml:"Token" yaml:"token" json:"token"`
}

type AIAgentConfig struct {
	// AutoRun settings for different command types
	View   bool `toml:"view,omitempty" yaml:"view,omitempty" json:"view,omitempty"`
	Edit   bool `toml:"edit,omitempty" yaml:"edit,omitempty" json:"edit,omitempty"`
	Delete bool `toml:"delete,omitempty" yaml:"delete,omitempty" json:"delete,omitempty"`
}

type AIConfig struct {
	Agent    AIAgentConfig `toml:"agent,omitempty" yaml:"agent,omitempty" json:"agent,omitempty"`
	ShowTips *bool         `toml:"showTips" yaml:"showTips" json:"showTips"`
}

type CCUsage struct {
	Enabled *bool `toml:"enabled" yaml:"enabled" json:"enabled"`
}

// CCOtel configuration for OTEL-based Claude Code tracking (v2)
type CCOtel struct {
	Enabled  *bool `toml:"enabled" yaml:"enabled" json:"enabled"`
	GRPCPort int   `toml:"grpcPort,omitempty" yaml:"grpcPort,omitempty" json:"grpcPort,omitempty"` // default: 54027
	Debug    *bool `toml:"debug" yaml:"debug" json:"debug"`                                        // write raw JSON to debug files
}

// CodeTracking configuration for coding activity heartbeat tracking
type CodeTracking struct {
	Enabled     *bool  `toml:"enabled" yaml:"enabled" json:"enabled"`
	APIEndpoint string `toml:"apiEndpoint,omitempty" yaml:"apiEndpoint,omitempty" json:"apiEndpoint,omitempty"` // Custom API endpoint for heartbeats
	Token       string `toml:"token,omitempty" yaml:"token,omitempty" json:"token,omitempty"`                   // Custom token for heartbeats
}

// LogCleanup configuration for automatic log file cleanup
type LogCleanup struct {
	Enabled     *bool `toml:"enabled" yaml:"enabled" json:"enabled"`                                           // default: true (enabled by default)
	ThresholdMB int64 `toml:"thresholdMB,omitempty" yaml:"thresholdMB,omitempty" json:"thresholdMB,omitempty"` // default: 100 MB
}

type ShellTimeConfig struct {
	Token       string `toml:"Token" yaml:"token" json:"token"`
	APIEndpoint string `toml:"APIEndpoint" yaml:"apiEndpoint,omitempty" json:"apiEndpoint,omitempty"`
	WebEndpoint string `toml:"WebEndpoint" yaml:"webEndpoint,omitempty" json:"webEndpoint,omitempty"`
	// how often sync to server
	FlushCount int `toml:"FlushCount,omitempty" yaml:"flushCount,omitempty" json:"flushCount,omitempty"`
	// how long the synced data would keep in db:
	// unit is days
	GCTime int `toml:"GCTime,omitempty" yaml:"gcTime,omitempty" json:"gcTime,omitempty"`

	// is data should be masking?
	// @default true
	DataMasking *bool `toml:"dataMasking,omitempty" yaml:"dataMasking,omitempty" json:"dataMasking,omitempty"`

	// for debug purpose
	Endpoints []Endpoint `toml:"ENDPOINTS,omitempty" yaml:"endpoints,omitempty" json:"endpoints,omitempty"`

	// WARNING
	// This config will track each command metrics you run in current shell.
	// Use this config only the developer asked you to do so.
	// This could be very slow on each command you run.
	EnableMetrics *bool `toml:"enableMetrics,omitempty" yaml:"enableMetrics,omitempty" json:"enableMetrics,omitempty"`

	Encrypted *bool `toml:"encrypted,omitempty" yaml:"encrypted,omitempty" json:"encrypted,omitempty"`

	// AI configuration
	AI *AIConfig `toml:"ai,omitempty" yaml:"ai,omitempty" json:"ai,omitempty"`

	// Exclude patterns - regular expressions to exclude commands from being saved
	// Commands matching any of these patterns will not be synced to the server
	Exclude []string `toml:"exclude,omitempty" yaml:"exclude,omitempty" json:"exclude,omitempty"`

	// CCUsage configuration for Claude Code usage tracking (v1 - ccusage CLI based)
	CCUsage *CCUsage `toml:"ccusage" yaml:"ccusage" json:"ccusage"`

	// CCOtel configuration for OTEL-based Claude Code tracking (v2 - gRPC passthrough)
	CCOtel *CCOtel `toml:"ccotel" yaml:"ccotel" json:"ccotel"`

	// CodeTracking configuration for coding activity heartbeat tracking
	CodeTracking *CodeTracking `toml:"codeTracking" yaml:"codeTracking" json:"codeTracking"`

	// LogCleanup configuration for automatic log file cleanup in daemon
	LogCleanup *LogCleanup `toml:"logCleanup" yaml:"logCleanup" json:"logCleanup"`

	// SocketPath is the path to the Unix domain socket used for communication
	// between the CLI and the daemon.
	SocketPath string `toml:"socketPath" yaml:"socketPath" json:"socketPath"`
}

var DefaultAIConfig = &AIConfig{
	Agent: AIAgentConfig{
		View:   false,
		Edit:   false,
		Delete: false,
	},
	ShowTips: nil, // defaults to true if nil
}

var DefaultConfig = ShellTimeConfig{
	Token:       "",
	APIEndpoint: "https://api.shelltime.xyz",
	WebEndpoint: "https://shelltime.xyz",
	FlushCount:  10,
	// 2 weeks by default
	GCTime:        14,
	DataMasking:   nil,
	Endpoints:     nil,
	EnableMetrics: nil,
	Encrypted:     nil,
	AI:            DefaultAIConfig,
	Exclude:       []string{},
	CCUsage:       nil,
	CCOtel:        nil,
	CodeTracking:  nil,
	LogCleanup:    nil,

	SocketPath: DefaultSocketPath,
}
