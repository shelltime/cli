package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

	"github.com/gookit/color"
)

// ClaudeSettingsAICodeOtelEnvService installs the Claude Code OTEL env vars into the `env` object of
// ~/.claude/settings.json. Claude Code applies that object to every session (terminal, desktop app,
// IDE), unlike shell rc exports, which only reach sessions launched from a shell.
type ClaudeSettingsAICodeOtelEnvService struct {
	settingsPath string
}

func NewClaudeSettingsAICodeOtelEnvService() *ClaudeSettingsAICodeOtelEnvService {
	return &ClaudeSettingsAICodeOtelEnvService{
		settingsPath: os.ExpandEnv("$HOME/.claude/settings.json"),
	}
}

func (s *ClaudeSettingsAICodeOtelEnvService) SettingsPath() string {
	return s.settingsPath
}

// claudeSettingsEnvVar is one managed entry of the settings.json `env` object.
type claudeSettingsEnvVar struct {
	Key   string
	Value string
}

// claudeSettingsOtelEnvVars returns the managed env vars in the order they are written.
// settings.json values are not shell-expanded, so OTEL_RESOURCE_ATTRIBUTES is resolved here.
func claudeSettingsOtelEnvVars() []claudeSettingsEnvVar {
	return []claudeSettingsEnvVar{
		{"CLAUDE_CODE_ENABLE_TELEMETRY", "1"},
		{"OTEL_METRICS_EXPORTER", "otlp"},
		{"OTEL_LOGS_EXPORTER", "otlp"},
		{"OTEL_EXPORTER_OTLP_PROTOCOL", "grpc"},
		{"OTEL_EXPORTER_OTLP_ENDPOINT", aiCodeOtelEndpoint},
		{"OTEL_METRIC_EXPORT_INTERVAL", "10000"},
		{"OTEL_LOGS_EXPORT_INTERVAL", "5000"},
		{"OTEL_LOG_USER_PROMPTS", "1"},
		{"OTEL_METRICS_INCLUDE_SESSION_ID", "true"},
		{"OTEL_METRICS_INCLUDE_VERSION", "true"},
		{"OTEL_METRICS_INCLUDE_ACCOUNT_UUID", "true"},
		{"OTEL_RESOURCE_ATTRIBUTES", claudeSettingsResourceAttributes()},
	}
}

func claudeSettingsResourceAttributes() string {
	username := os.Getenv("USER")
	if u, err := user.Current(); err == nil && u.Username != "" {
		username = u.Username
	}
	hostname, _ := os.Hostname()
	return fmt.Sprintf("user.name=%s,machine.name=%s,team.id=shelltime", username, hostname)
}

func (s *ClaudeSettingsAICodeOtelEnvService) Install() error {
	data, mode, err := s.readSettings()
	if err != nil {
		return err
	}

	root, err := decodeOrderedObject(data)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", s.settingsPath, err)
	}

	var env []jsonField
	if raw, ok := getField(root, "env"); ok {
		if env, err = decodeOrderedObject(raw); err != nil {
			return fmt.Errorf("failed to parse env in %s: %w", s.settingsPath, err)
		}
	}

	for _, v := range claudeSettingsOtelEnvVars() {
		value, err := json.Marshal(v.Value)
		if err != nil {
			return err
		}
		env = setField(env, v.Key, value)
	}
	root = setField(root, "env", encodeOrderedObject(env))

	if err := os.MkdirAll(filepath.Dir(s.settingsPath), 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(s.settingsPath), err)
	}
	if err := s.writeSettings(root, mode); err != nil {
		return err
	}

	color.Green.Printf("Claude Code OTEL config installed in %s\n", s.settingsPath)
	return nil
}

func (s *ClaudeSettingsAICodeOtelEnvService) Uninstall() error {
	data, mode, err := s.readSettings()
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}

	root, err := decodeOrderedObject(data)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", s.settingsPath, err)
	}
	raw, ok := getField(root, "env")
	if !ok {
		return nil
	}
	env, err := decodeOrderedObject(raw)
	if err != nil {
		return fmt.Errorf("failed to parse env in %s: %w", s.settingsPath, err)
	}

	removed := false
	for _, v := range claudeSettingsOtelEnvVars() {
		var found bool
		if env, found = deleteField(env, v.Key); found {
			removed = true
		}
	}
	if !removed {
		return nil
	}

	if len(env) == 0 {
		root, _ = deleteField(root, "env")
	} else {
		root = setField(root, "env", encodeOrderedObject(env))
	}
	return s.writeSettings(root, mode)
}

func (s *ClaudeSettingsAICodeOtelEnvService) Check() error {
	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		return fmt.Errorf("claude settings file not found at %s", s.settingsPath)
	}

	var settings struct {
		Env map[string]any `json:"env"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("failed to parse %s: %w", s.settingsPath, err)
	}

	if _, ok := settings.Env["CLAUDE_CODE_ENABLE_TELEMETRY"]; !ok || settings.Env["OTEL_EXPORTER_OTLP_ENDPOINT"] != aiCodeOtelEndpoint {
		return fmt.Errorf("Claude Code OTEL config not found in %s", s.settingsPath)
	}
	return nil
}

// readSettings returns the settings file content and mode. A missing file yields nil data.
func (s *ClaudeSettingsAICodeOtelEnvService) readSettings() ([]byte, os.FileMode, error) {
	info, err := os.Stat(s.settingsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0644, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("failed to stat %s: %w", s.settingsPath, err)
	}

	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read %s: %w", s.settingsPath, err)
	}
	return data, info.Mode().Perm(), nil
}

// writeSettings writes via a temp file and rename so a concurrent reader (Claude Code) never sees a
// partially written file.
func (s *ClaudeSettingsAICodeOtelEnvService) writeSettings(root []jsonField, mode os.FileMode) error {
	var out bytes.Buffer
	if err := json.Indent(&out, encodeOrderedObject(root), "", "  "); err != nil {
		return fmt.Errorf("failed to format settings: %w", err)
	}
	out.WriteByte('\n')

	tmp, err := os.CreateTemp(filepath.Dir(s.settingsPath), ".settings.json.*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(out.Bytes()); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.settingsPath); err != nil {
		return fmt.Errorf("failed to write %s: %w", s.settingsPath, err)
	}
	return nil
}

// jsonField is one key/value pair of a JSON object, kept in document order. encoding/json maps
// sort keys, which would reshuffle a user-edited settings.json (often tracked in dotfiles).
type jsonField struct {
	Key   string
	Value json.RawMessage
}

// decodeOrderedObject decodes a JSON object into its fields in document order.
// Empty input and `null` are treated as an empty object.
func decodeOrderedObject(data []byte) ([]jsonField, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected a JSON object")
	}

	var fields []jsonField
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected an object key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		fields = append(fields, jsonField{Key: key, Value: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after the JSON object")
	}
	return fields, nil
}

// encodeOrderedObject encodes fields as a compact JSON object in their current order.
func encodeOrderedObject(fields []jsonField) json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := json.Marshal(f.Key)
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(f.Value)
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

func getField(fields []jsonField, key string) (json.RawMessage, bool) {
	for _, f := range fields {
		if f.Key == key {
			return f.Value, true
		}
	}
	return nil, false
}

// setField replaces the value of key in place, or appends the key when it is absent.
func setField(fields []jsonField, key string, value json.RawMessage) []jsonField {
	for i := range fields {
		if fields[i].Key == key {
			fields[i].Value = value
			return fields
		}
	}
	return append(fields, jsonField{Key: key, Value: value})
}

func deleteField(fields []jsonField, key string) ([]jsonField, bool) {
	for i, f := range fields {
		if f.Key == key {
			return append(fields[:i], fields[i+1:]...), true
		}
	}
	return fields, false
}
