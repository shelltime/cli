package commands

import (
	"context"
	"os"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
)

const aiCloudOnlyReason = "AI cloud only mode collects only AI coding agent usage on this machine " +
	"(aiCloudOnly in the config, on by default in a Claude Code cloud environment)"

// skippedInAICloudOnly reports whether AI cloud only mode is on for cfg, after saying which
// action it skipped. Commands that would upload commands or anything about the machine stop then.
func skippedInAICloudOnly(cfg model.ShellTimeConfig, action string) bool {
	if !model.IsAICloudOnly(cfg, os.Getenv) {
		return false
	}
	color.Yellow.Printf("%s: %s.\n", action, aiCloudOnlyReason)
	return true
}

// configForAICloudOnly reads the config for a command that can run before signing in, so there
// may be none yet; the environment alone decides the mode then.
func configForAICloudOnly(ctx context.Context) model.ShellTimeConfig {
	cfg, err := model.NewConfigService(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER)).ReadConfigFile(ctx)
	if err != nil {
		return model.ShellTimeConfig{}
	}
	return cfg
}
