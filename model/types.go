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
	View   bool `toml:"view" yaml:"view" json:"view"`
	Edit   bool `toml:"edit" yaml:"edit" json:"edit"`
	Delete bool `toml:"delete" yaml:"delete" json:"delete"`
}

type AIConfig struct {
	Agent    AIAgentConfig `toml:"agent" yaml:"agent" json:"agent"`
	ShowTips *bool         `toml:"showTips" yaml:"showTips" json:"showTips"`
}

type CCUsage struct {
	Enabled *bool `toml:"enabled" yaml:"enabled" json:"enabled"`
}

// CCOtel configuration for OTEL-based Claude Code tracking (v2)
type CCOtel struct {
	Enabled  *bool `toml:"enabled" yaml:"enabled" json:"enabled"`
	GRPCPort int   `toml:"grpcPort" yaml:"grpcPort" json:"grpcPort"` // default: 4317
	Debug    *bool `toml:"debug" yaml:"debug" json:"debug"`          // write raw JSON to debug files
}

// CodeTracking configuration for coding activity heartbeat tracking
type CodeTracking struct {
	Enabled     *bool  `toml:"enabled" yaml:"enabled" json:"enabled"`
	APIEndpoint string `toml:"apiEndpoint" yaml:"apiEndpoint" json:"apiEndpoint"` // Custom API endpoint for heartbeats
	Token       string `toml:"token" yaml:"token" json:"token"`                   // Custom token for heartbeats
}

// LogCleanup configuration for automatic log file cleanup
type LogCleanup struct {
	Enabled     *bool `toml:"enabled" yaml:"enabled" json:"enabled"`             // default: true (enabled by default)
	ThresholdMB int64 `toml:"thresholdMB" yaml:"thresholdMB" json:"thresholdMB"` // default: 100 MB
}

type ShellTimeConfig struct {
	Token       string `toml:"Token" yaml:"token" json:"token"`
	APIEndpoint string `toml:"APIEndpoint" yaml:"apiEndpoint" json:"apiEndpoint"`
	WebEndpoint string `toml:"WebEndpoint" yaml:"webEndpoint" json:"webEndpoint"`
	// how often sync to server
	FlushCount int `toml:"FlushCount" yaml:"flushCount" json:"flushCount"`
	// how long the synced data would keep in db:
	// unit is days
	GCTime int `toml:"GCTime" yaml:"gcTime" json:"gcTime"`

	// is data should be masking?
	// @default true
	DataMasking *bool `toml:"dataMasking" yaml:"dataMasking" json:"dataMasking"`

	// for debug purpose
	Endpoints []Endpoint `toml:"ENDPOINTS" yaml:"endpoints" json:"endpoints"`

	// WARNING
	// This config will track each command metrics you run in current shell.
	// Use this config only the developer asked you to do so.
	// This could be very slow on each command you run.
	EnableMetrics *bool `toml:"enableMetrics" yaml:"enableMetrics" json:"enableMetrics"`

	Encrypted *bool `toml:"encrypted" yaml:"encrypted" json:"encrypted"`

	// AI configuration
	AI *AIConfig `toml:"ai" yaml:"ai" json:"ai"`

	// Exclude patterns - regular expressions to exclude commands from being saved
	// Commands matching any of these patterns will not be synced to the server
	Exclude []string `toml:"exclude" yaml:"exclude" json:"exclude"`

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
