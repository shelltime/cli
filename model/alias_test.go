package model

import (
	"encoding/json"
	"testing"
)

func TestAlias_JSON(t *testing.T) {
	alias := Alias{
		Name:  "ll",
		Value: "ls -la",
		Shell: "bash",
	}

	data, err := json.Marshal(alias)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded Alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Name != alias.Name {
		t.Errorf("Name mismatch: expected %s, got %s", alias.Name, decoded.Name)
	}
	if decoded.Value != alias.Value {
		t.Errorf("Value mismatch: expected %s, got %s", alias.Value, decoded.Value)
	}
	if decoded.Shell != alias.Shell {
		t.Errorf("Shell mismatch: expected %s, got %s", alias.Shell, decoded.Shell)
	}
}

func TestImportShellAliasRequest_JSON(t *testing.T) {
	req := importShellAliasRequest{
		Aliases:       []string{"alias ll='ls -la'", "alias gs='git status'"},
		IsFullRefresh: true,
		ShellType:     "bash",
		FileLocation:  "~/.bashrc",
		Hostname:      "myhost",
		Username:      "myuser",
		OS:            "linux",
		OSVersion:     "ubuntu 22.04",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded importShellAliasRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Aliases) != 2 {
		t.Errorf("Expected 2 aliases, got %d", len(decoded.Aliases))
	}
	if decoded.IsFullRefresh != true {
		t.Error("IsFullRefresh should be true")
	}
	if decoded.ShellType != "bash" {
		t.Errorf("ShellType mismatch: expected bash, got %s", decoded.ShellType)
	}
	if decoded.FileLocation != "~/.bashrc" {
		t.Errorf("FileLocation mismatch")
	}
	if decoded.Hostname != "myhost" {
		t.Errorf("Hostname mismatch")
	}
	if decoded.Username != "myuser" {
		t.Errorf("Username mismatch")
	}
	if decoded.OS != "linux" {
		t.Errorf("OS mismatch")
	}
	if decoded.OSVersion != "ubuntu 22.04" {
		t.Errorf("OSVersion mismatch")
	}
}

func TestImportShellAliasResponse_JSON(t *testing.T) {
	resp := importShellAliasResponse{
		Success: true,
		Count:   5,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded importShellAliasResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Success != true {
		t.Error("Success should be true")
	}
	if decoded.Count != 5 {
		t.Errorf("Count mismatch: expected 5, got %d", decoded.Count)
	}
}

func TestImportShellAliasRequest_EmptyAliases(t *testing.T) {
	req := importShellAliasRequest{
		Aliases:       []string{},
		IsFullRefresh: false,
		ShellType:     "zsh",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded importShellAliasRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Aliases) != 0 {
		t.Errorf("Expected 0 aliases, got %d", len(decoded.Aliases))
	}
}

func TestAlias_EmptyValues(t *testing.T) {
	alias := Alias{
		Name:  "",
		Value: "",
		Shell: "",
	}

	data, err := json.Marshal(alias)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded Alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Name != "" {
		t.Errorf("Expected empty name")
	}
	if decoded.Value != "" {
		t.Errorf("Expected empty value")
	}
	if decoded.Shell != "" {
		t.Errorf("Expected empty shell")
	}
}

func TestAlias_SpecialCharacters(t *testing.T) {
	testCases := []struct {
		name  string
		alias Alias
	}{
		{
			"quotes in value",
			Alias{Name: "greet", Value: `echo "hello world"`, Shell: "bash"},
		},
		{
			"pipes and redirects",
			Alias{Name: "findlog", Value: "find . -name '*.log' | xargs grep error", Shell: "bash"},
		},
		{
			"unicode characters",
			Alias{Name: "emoji", Value: "echo '🚀 Starting...'", Shell: "zsh"},
		},
		{
			"backslashes",
			Alias{Name: "path", Value: `echo "C:\Users\test"`, Shell: "bash"},
		},
		{
			"newlines escaped",
			Alias{Name: "multiline", Value: "echo 'line1\\nline2'", Shell: "bash"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.alias)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var decoded Alias
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if decoded.Value != tc.alias.Value {
				t.Errorf("Value mismatch: expected %q, got %q", tc.alias.Value, decoded.Value)
			}
		})
	}
}

func TestImportShellAliasRequest_ShellTypes(t *testing.T) {
	shells := []string{"bash", "zsh", "fish", "sh", "ksh"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			req := importShellAliasRequest{
				Aliases:   []string{"alias test='echo test'"},
				ShellType: shell,
			}

			data, _ := json.Marshal(req)
			var decoded importShellAliasRequest
			json.Unmarshal(data, &decoded)

			if decoded.ShellType != shell {
				t.Errorf("ShellType mismatch: expected %s, got %s", shell, decoded.ShellType)
			}
		})
	}
}

func TestImportShellAliasResponse_FailureCase(t *testing.T) {
	resp := importShellAliasResponse{
		Success: false,
		Count:   0,
	}

	data, _ := json.Marshal(resp)
	var decoded importShellAliasResponse
	json.Unmarshal(data, &decoded)

	if decoded.Success {
		t.Error("Expected success to be false")
	}
	if decoded.Count != 0 {
		t.Error("Expected count to be 0")
	}
}

func TestImportShellAliasRequest_FileLocations(t *testing.T) {
	locations := []string{
		"~/.bashrc",
		"~/.bash_profile",
		"~/.zshrc",
		"~/.config/fish/config.fish",
		"/etc/bash.bashrc",
	}

	for _, loc := range locations {
		t.Run(loc, func(t *testing.T) {
			req := importShellAliasRequest{
				Aliases:      []string{"alias test='test'"},
				FileLocation: loc,
			}

			data, _ := json.Marshal(req)
			var decoded importShellAliasRequest
			json.Unmarshal(data, &decoded)

			if decoded.FileLocation != loc {
				t.Errorf("FileLocation mismatch: expected %s, got %s", loc, decoded.FileLocation)
			}
		})
	}
}

func TestAlias_LongValue(t *testing.T) {
	// Create a very long alias value
	longValue := ""
	for i := 0; i < 1000; i++ {
		longValue += "echo 'test' && "
	}
	longValue += "echo 'done'"

	alias := Alias{
		Name:  "longcmd",
		Value: longValue,
		Shell: "bash",
	}

	data, err := json.Marshal(alias)
	if err != nil {
		t.Fatalf("Failed to marshal long alias: %v", err)
	}

	var decoded Alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal long alias: %v", err)
	}

	if decoded.Value != longValue {
		t.Error("Long value was not preserved correctly")
	}
}
