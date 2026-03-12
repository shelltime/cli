package commands

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gookit/color"
	"github.com/urfave/cli/v2"
)

// claudeCodeHookEvents lists all hook events to register for Claude Code
var claudeCodeHookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"PostToolUse",
	"PostToolUseFailure",
	"Stop",
	"SubagentStart",
	"SubagentStop",
	"SessionEnd",
	"Notification",
	"PreCompact",
}

// hookEntry represents a single hook configuration entry
type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async"`
}

var AICodeHooksInstallCommand = &cli.Command{
	Name:    "install",
	Aliases: []string{"i"},
	Usage:   "Install AI coding tool hooks (Claude Code, Codex, Cursor)",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "claude-code",
			Usage: "Install hooks for Claude Code only",
		},
		&cli.BoolFlag{
			Name:  "codex",
			Usage: "Install hooks for Codex only",
		},
		&cli.BoolFlag{
			Name:  "cursor",
			Usage: "Install hooks for Cursor only",
		},
		&cli.BoolFlag{
			Name:  "all",
			Value: true,
			Usage: "Install hooks for all supported tools",
		},
	},
	Action: commandAICodeHooksInstall,
}

var AICodeHooksUninstallCommand = &cli.Command{
	Name:    "uninstall",
	Aliases: []string{"u"},
	Usage:   "Uninstall AI coding tool hooks (Claude Code, Codex, Cursor)",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "claude-code",
			Usage: "Uninstall hooks for Claude Code only",
		},
		&cli.BoolFlag{
			Name:  "codex",
			Usage: "Uninstall hooks for Codex only",
		},
		&cli.BoolFlag{
			Name:  "cursor",
			Usage: "Uninstall hooks for Cursor only",
		},
		&cli.BoolFlag{
			Name:  "all",
			Value: true,
			Usage: "Uninstall hooks for all supported tools",
		},
	},
	Action: commandAICodeHooksUninstall,
}

func commandAICodeHooksInstall(c *cli.Context) error {
	installClaudeCode := c.Bool("claude-code")
	installCodex := c.Bool("codex")
	installCursor := c.Bool("cursor")
	installAll := c.Bool("all")

	// If specific tools are selected, don't install all
	if installClaudeCode || installCodex || installCursor {
		installAll = false
	}

	color.Yellow.Println("Installing AI code hooks...")

	if installAll || installClaudeCode {
		if err := installClaudeCodeHooks(); err != nil {
			color.Red.Printf("Failed to install Claude Code hooks: %v\n", err)
		} else {
			color.Green.Println("Claude Code hooks installed successfully")
		}
	}

	if installAll || installCodex {
		if err := installCodexHooks(); err != nil {
			color.Red.Printf("Failed to install Codex hooks: %v\n", err)
		} else {
			color.Green.Println("Codex hooks installed successfully")
		}
	}

	if installAll || installCursor {
		if err := installCursorHooks(); err != nil {
			color.Red.Printf("Failed to install Cursor hooks: %v\n", err)
		} else {
			color.Green.Println("Cursor hooks installed successfully")
		}
	}

	color.Green.Println("AI code hooks installation complete!")
	return nil
}

func commandAICodeHooksUninstall(c *cli.Context) error {
	uninstallClaudeCode := c.Bool("claude-code")
	uninstallCodex := c.Bool("codex")
	uninstallCursor := c.Bool("cursor")
	uninstallAll := c.Bool("all")

	// If specific tools are selected, don't uninstall all
	if uninstallClaudeCode || uninstallCodex || uninstallCursor {
		uninstallAll = false
	}

	color.Yellow.Println("Uninstalling AI code hooks...")

	if uninstallAll || uninstallClaudeCode {
		if err := uninstallClaudeCodeHooks(); err != nil {
			color.Red.Printf("Failed to uninstall Claude Code hooks: %v\n", err)
		} else {
			color.Green.Println("Claude Code hooks uninstalled successfully")
		}
	}

	if uninstallAll || uninstallCodex {
		if err := uninstallCodexHooks(); err != nil {
			color.Red.Printf("Failed to uninstall Codex hooks: %v\n", err)
		} else {
			color.Green.Println("Codex hooks uninstalled successfully")
		}
	}

	if uninstallAll || uninstallCursor {
		if err := uninstallCursorHooks(); err != nil {
			color.Red.Printf("Failed to uninstall Cursor hooks: %v\n", err)
		} else {
			color.Green.Println("Cursor hooks uninstalled successfully")
		}
	}

	color.Green.Println("AI code hooks uninstallation complete!")
	return nil
}

// installClaudeCodeHooks reads ~/.claude/settings.json, merges hooks, and writes back
func installClaudeCodeHooks() error {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")

	// Read existing settings or create new
	settings := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			slog.Error("Failed to parse Claude Code settings", slog.Any("err", err))
			return err
		}
	}

	// Get or create hooks map
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
	}

	shelltimeHook := hookEntry{
		Type:    "command",
		Command: "shelltime aicode-hooks",
		Async:   true,
	}

	// Add hooks for each event
	for _, eventName := range claudeCodeHookEvents {
		existingHooks, ok := hooks[eventName].([]any)
		if !ok {
			existingHooks = make([]any, 0)
		}

		// Check if shelltime hook already exists
		alreadyExists := false
		for _, h := range existingHooks {
			if hMap, ok := h.(map[string]any); ok {
				if cmd, ok := hMap["command"].(string); ok && cmd == "shelltime aicode-hooks" {
					alreadyExists = true
					break
				}
			}
		}

		if !alreadyExists {
			// Convert hookEntry to map for JSON compatibility
			hookMap := map[string]any{
				"type":    shelltimeHook.Type,
				"command": shelltimeHook.Command,
				"async":   shelltimeHook.Async,
			}
			existingHooks = append(existingHooks, hookMap)
			hooks[eventName] = existingHooks
		}
	}

	settings["hooks"] = hooks

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}

	// Write back
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

// uninstallClaudeCodeHooks removes shelltime hooks from ~/.claude/settings.json
func uninstallClaudeCodeHooks() error {
	settingsPath := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to uninstall
		}
		return err
	}

	settings := make(map[string]any)
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil // No hooks to remove
	}

	for _, eventName := range claudeCodeHookEvents {
		existingHooks, ok := hooks[eventName].([]any)
		if !ok {
			continue
		}

		// Filter out shelltime hooks
		filteredHooks := make([]any, 0)
		for _, h := range existingHooks {
			if hMap, ok := h.(map[string]any); ok {
				if cmd, ok := hMap["command"].(string); ok && cmd == "shelltime aicode-hooks" {
					continue // Skip shelltime hooks
				}
			}
			filteredHooks = append(filteredHooks, h)
		}

		if len(filteredHooks) == 0 {
			delete(hooks, eventName)
		} else {
			hooks[eventName] = filteredHooks
		}
	}

	settings["hooks"] = hooks

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, output, 0644)
}

// installCodexHooks creates/updates ~/.codex/hooks.json
func installCodexHooks() error {
	hooksPath := filepath.Join(os.Getenv("HOME"), ".codex", "hooks.json")
	return installGenericHooks(hooksPath, "codex")
}

// uninstallCodexHooks removes shelltime hooks from ~/.codex/hooks.json
func uninstallCodexHooks() error {
	hooksPath := filepath.Join(os.Getenv("HOME"), ".codex", "hooks.json")
	return uninstallGenericHooks(hooksPath)
}

// installCursorHooks creates/updates ~/.cursor/hooks.json
func installCursorHooks() error {
	hooksPath := filepath.Join(os.Getenv("HOME"), ".cursor", "hooks.json")
	return installGenericHooks(hooksPath, "cursor")
}

// uninstallCursorHooks removes shelltime hooks from ~/.cursor/hooks.json
func uninstallCursorHooks() error {
	hooksPath := filepath.Join(os.Getenv("HOME"), ".cursor", "hooks.json")
	return uninstallGenericHooks(hooksPath)
}

// installGenericHooks creates/updates a hooks.json file for Codex/Cursor style tools
func installGenericHooks(hooksPath string, source string) error {
	hooksConfig := make(map[string]any)
	if data, err := os.ReadFile(hooksPath); err == nil {
		if err := json.Unmarshal(data, &hooksConfig); err != nil {
			slog.Error("Failed to parse hooks config", slog.Any("err", err), slog.String("path", hooksPath))
			return err
		}
	}

	// Get or create hooks array
	hooks, ok := hooksConfig["hooks"].([]any)
	if !ok {
		hooks = make([]any, 0)
	}

	// Check if shelltime hook already exists
	for _, h := range hooks {
		if hMap, ok := h.(map[string]any); ok {
			if cmd, ok := hMap["command"].(string); ok && cmd == "shelltime aicode-hooks --source="+source {
				return nil // Already installed
			}
		}
	}

	hookMap := map[string]any{
		"type":    "command",
		"command": "shelltime aicode-hooks --source=" + source,
		"async":   true,
	}
	hooks = append(hooks, hookMap)
	hooksConfig["hooks"] = hooks

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(hooksConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(hooksPath, data, 0644)
}

// uninstallGenericHooks removes shelltime hooks from a hooks.json file
func uninstallGenericHooks(hooksPath string) error {
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	hooksConfig := make(map[string]any)
	if err := json.Unmarshal(data, &hooksConfig); err != nil {
		return err
	}

	hooks, ok := hooksConfig["hooks"].([]any)
	if !ok {
		return nil
	}

	filteredHooks := make([]any, 0)
	for _, h := range hooks {
		if hMap, ok := h.(map[string]any); ok {
			if cmd, ok := hMap["command"].(string); ok {
				if cmd == "shelltime aicode-hooks" ||
					cmd == "shelltime aicode-hooks --source=codex" ||
					cmd == "shelltime aicode-hooks --source=cursor" {
					continue // Skip shelltime hooks
				}
			}
		}
		filteredHooks = append(filteredHooks, h)
	}

	hooksConfig["hooks"] = filteredHooks

	output, err := json.MarshalIndent(hooksConfig, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(hooksPath, output, 0644)
}
