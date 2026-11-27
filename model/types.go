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
	// ShareContext controls whether `shelltime q` sends context alongside the
	// prompt: working directory, hostname, git state, project tooling,
	// installed tools, a directory listing and machine info. Defaults to true
	// if unset; when false only the shell, OS and prompt are sent.
	ShareContext *bool `toml:"shareContext,omitempty" yaml:"shareContext,omitempty" json:"shareContext,omitempty"`
}

// AICodeOtel configuration for OTEL-based AI CLI tracking (Claude Code, Codex, etc.)
// The processor auto-detects the source from service.name attribute
type AICodeOtel struct {
	Enabled  *bool `toml:"enabled" yaml:"enabled" json:"enabled"`
	GRPCPort int   `toml:"grpcPort,omitempty" yaml:"grpcPort,omitempty" json:"grpcPort,omitempty"` // default: 54027
	Debug    *bool `toml:"debug" yaml:"debug" json:"debug"`                                        // write raw JSON to debug files
	// SendRepository names the git repository of each coding agent session's folder, so the
	// server groups sessions by repository (default true). False: ShellTime never asks git or
	// sends a repository, and the daemon removes the one Claude Code reports, so sessions are
	// grouped by machine and folder.
	SendRepository *bool `toml:"sendRepository,omitempty" yaml:"sendRepository,omitempty" json:"sendRepository,omitempty"`
}

// SendsRepository reports whether AI coding agent data may name its git repository: on unless
// aiCodeOtel.sendRepository is false.
func (c ShellTimeConfig) SendsRepository() bool {
	return c.AICodeOtel == nil || c.AICodeOtel.SendRepository == nil || *c.AICodeOtel.SendRepository
}

// AgentHooksConfig configures the hooks of coding agents without OTEL (Cursor, Antigravity):
// `shelltime cursor install` and `shelltime agy install`.
type AgentHooksConfig struct {
	// SendContent sends the text of prompts, agent responses and tool input (default true).
	// False sends only the activity: event types, tool names, commands, file paths, timing.
	SendContent *bool `toml:"sendContent,omitempty" yaml:"sendContent,omitempty" json:"sendContent,omitempty"`
}

// AgentHooksSendContent reports whether agent hook events may carry prompt, response and tool
// input text: on unless agentHooks.sendContent is false.
func (c ShellTimeConfig) AgentHooksSendContent() bool {
	return c.AgentHooks == nil || c.AgentHooks.SendContent == nil || *c.AgentHooks.SendContent
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

	// AICodeOtel configuration for OTEL-based AI CLI tracking (Claude Code, Codex, etc.)
	AICodeOtel *AICodeOtel `toml:"aiCodeOtel" yaml:"aiCodeOtel" json:"aiCodeOtel"`

	// CodeTracking configuration for coding activity heartbeat tracking
	CodeTracking *CodeTracking `toml:"codeTracking" yaml:"codeTracking" json:"codeTracking"`

	// AgentHooks configures what the Cursor and Antigravity hooks send
	AgentHooks *AgentHooksConfig `toml:"agentHooks,omitempty" yaml:"agentHooks,omitempty" json:"agentHooks,omitempty"`

	// AICloudOnly collects only AI coding agent telemetry (aiCodeOtel) and nothing about the
	// machine: no shell commands, editor heartbeats, aliases or dotfiles, so it never becomes a
	// host. Unset, it is on in a cloud agent's VM, which isn't one of the user's machines: a
	// Claude Code cloud environment (CLAUDE_CODE_REMOTE=true), or one that sets
	// SHELLTIME_CLOUD=true. See IsAICloudOnly.
	AICloudOnly *bool `toml:"aiCloudOnly,omitempty" yaml:"aiCloudOnly,omitempty" json:"aiCloudOnly,omitempty"`

	// LogCleanup configuration for automatic log file cleanup in daemon
	LogCleanup *LogCleanup `toml:"logCleanup" yaml:"logCleanup" json:"logCleanup"`

	// Storage selects the local command buffering backend. When unset the
	// always-available txt file store is used.
	Storage *StorageConfig `toml:"storage" yaml:"storage,omitempty" json:"storage,omitempty"`

	// Proxy routes all outbound HTTP(S) traffic through a proxy.
	// When unset, the HTTP_PROXY / HTTPS_PROXY / NO_PROXY env vars are honored.
	Proxy *ProxyConfig `toml:"proxy,omitempty" yaml:"proxy,omitempty" json:"proxy,omitempty"`

	// SocketPath is the path to the Unix domain socket used for communication
	// between the CLI and the daemon.
	SocketPath string `toml:"socketPath" yaml:"socketPath" json:"socketPath"`
}

// ProxyConfig configures the proxy used for outbound HTTP(S) requests.
type ProxyConfig struct {
	// URL of the proxy. Supported schemes: http, https, socks5, socks5h.
	// Credentials may be embedded, e.g. socks5://user:pass@127.0.0.1:1080.
	// A bare "host:port" is treated as http://host:port.
	URL string `toml:"url" yaml:"url" json:"url"`
	// NoProxy lists hosts that bypass the proxy, using NO_PROXY syntax:
	// "localhost", ".internal.corp", "10.0.0.0/8", "*.example.com".
	NoProxy []string `toml:"noProxy,omitempty" yaml:"noProxy,omitempty" json:"noProxy,omitempty"`
}

// StorageConfig selects which CommandStore backend buffers tracked commands
// before they sync to the server.
type StorageConfig struct {
	// Engine is "file" (default) or "bolt". The bolt engine is daemon-owned;
	// see CommandStore for details.
	Engine string `toml:"engine" yaml:"engine" json:"engine"`
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
	DataMasking:   new(true),
	Endpoints:     nil,
	EnableMetrics: nil,
	Encrypted:     new(true),
	AI:            DefaultAIConfig,
	Exclude:       []string{},
	AICodeOtel: new(AICodeOtel{
		Enabled:  new(true),
		GRPCPort: DefaultAICodeOtelGRPCPort,
		Debug:    new(false),
	}),
	CodeTracking: new(CodeTracking{
		Enabled: new(true),
	}),
	LogCleanup: nil,

	SocketPath: DefaultSocketPath,
}
