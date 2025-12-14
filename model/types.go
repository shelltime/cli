package model

const (
	DefaultSocketPath = "/tmp/shelltime.sock"
)

type Endpoint struct {
	APIEndpoint string `toml:"apiEndpoint"`
	Token       string `token:"token"`
}

type AIAgentConfig struct {
	// AutoRun settings for different command types
	View   bool `toml:"view"`
	Edit   bool `toml:"edit"`
	Delete bool `toml:"delete"`
}

type AIConfig struct {
	Agent    AIAgentConfig `toml:"agent"`
	ShowTips *bool         `toml:"showTips"`
}

type CCUsage struct {
	Enabled *bool `toml:"enabled"`
}

// CCOtel configuration for OTEL-based Claude Code tracking (v2)
type CCOtel struct {
	Enabled  *bool `toml:"enabled"`
	GRPCPort int   `toml:"grpcPort"` // default: 4317
}

type ShellTimeConfig struct {
	Token       string
	APIEndpoint string
	WebEndpoint string
	// how often sync to server
	FlushCount int
	// how long the synced data would keep in db:
	// unit is days
	GCTime int

	// is data should be masking?
	// @default true
	DataMasking *bool `toml:"dataMasking"`

	// for debug purpose
	Endpoints []Endpoint `toml:"ENDPOINTS"`

	// WARNING
	// This config will track each command metrics you run in current shell.
	// Use this config only the developer asked you to do so.
	// This could be very slow on each command you run.
	EnableMetrics *bool `toml:"enableMetrics"`

	Encrypted *bool `toml:"encrypted"`

	// AI configuration
	AI *AIConfig `toml:"ai"`

	// Exclude patterns - regular expressions to exclude commands from being saved
	// Commands matching any of these patterns will not be synced to the server
	Exclude []string `toml:"exclude"`

	// CCUsage configuration for Claude Code usage tracking (v1 - ccusage CLI based)
	CCUsage *CCUsage `toml:"ccusage"`

	// CCOtel configuration for OTEL-based Claude Code tracking (v2 - gRPC passthrough)
	CCOtel *CCOtel `toml:"ccotel"`

	// SocketPath is the path to the Unix domain socket used for communication
	// between the CLI and the daemon.
	SocketPath string `toml:"socketPath"`
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

	SocketPath: DefaultSocketPath,
}
