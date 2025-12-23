package model

// HeartbeatPayload represents the payload for heartbeat ingestion
// Sent by client daemon in batches
type HeartbeatPayload struct {
	Heartbeats []HeartbeatData `json:"heartbeats"`
}

// HeartbeatData represents a single heartbeat event
type HeartbeatData struct {
	// Unique identifier for idempotency (client-generated UUID)
	HeartbeatID string `json:"heartbeatId"`

	// Core activity data
	Entity     string `json:"entity"`               // File path, URL, or app name
	EntityType string `json:"entityType,omitempty"` // "file", "app", "domain" - defaults to "file"
	Category   string `json:"category,omitempty"`   // "coding", "debugging", etc. - defaults to "coding"
	Time       int64  `json:"time"`                 // Unix timestamp in seconds

	// Project context
	Project         string `json:"project,omitempty"`
	ProjectRootPath string `json:"projectRootPath,omitempty"`
	Branch          string `json:"branch,omitempty"`

	// File details
	Language       string `json:"language,omitempty"`
	Lines          *int   `json:"lines,omitempty"`
	LineNumber     *int   `json:"lineNumber,omitempty"`
	CursorPosition *int   `json:"cursorPosition,omitempty"`

	// Editor/IDE information
	Editor        string `json:"editor,omitempty"`
	EditorVersion string `json:"editorVersion,omitempty"`
	Plugin        string `json:"plugin,omitempty"`
	PluginVersion string `json:"pluginVersion,omitempty"`

	// Machine context
	Machine   string `json:"machine,omitempty"`
	OS        string `json:"os,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`

	// Write tracking
	IsWrite bool `json:"isWrite,omitempty"`
}

// HeartbeatResponse represents the response for heartbeat ingestion
type HeartbeatResponse struct {
	Success   bool   `json:"success"`
	Processed int    `json:"processed"`
	Errors    int    `json:"errors"`
	Message   string `json:"message,omitempty"`
}
