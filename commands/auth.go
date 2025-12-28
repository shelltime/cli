package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/gookit/color"
	"github.com/invopop/jsonschema"
	"github.com/malamtime/cli/model"
	"github.com/pkg/browser"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/yaml.v3"
)

var AuthCommand *cli.Command = &cli.Command{
	Name:  "auth",
	Usage: "Authenticate with shelltime.xyz",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "token",
			Aliases:  []string{"t"},
			Usage:    "Authentication token",
			Required: false,
		},
	},
	Action: commandAuth,
}

func commandAuth(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "auth")
	defer span.End()
	configDir := os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER)
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		err = os.Mkdir(configDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	}
	SetupLogger(configDir)

	// Generate JSON schema for IDE autocompletion
	schemaFile := configDir + "/config-schema.json"
	if err := generateSchemaFile(schemaFile); err != nil {
		slog.Warn("Failed to generate schema file", slog.Any("err", err))
	}

	var config model.ShellTimeConfig
	configFile := configDir + "/config.yaml"
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		content, err := yaml.Marshal(model.DefaultConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal default config: %w", err)
		}
		// Prepend $schema for IDE autocompletion support
		schemaHeader := "# yaml-language-server: $schema=./config-schema.json\n"
		content = append([]byte(schemaHeader), content...)
		err = os.WriteFile(configFile, content, 0644)
		if err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}
		config = model.DefaultConfig
	} else {
		existingConfig, err := configService.ReadConfigFile(ctx)
		if err != nil {
			return err
		}
		config = existingConfig
	}

	newToken := c.String("token")

	if newToken == "" {
		nt, err := ApplyTokenByHandshake(ctx, config)
		if err != nil {
			return err
		}
		newToken = nt
	}

	config.Token = newToken
	content, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	err = os.WriteFile(configFile, content, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	color.Green.Println(" ✅ config file created")
	return nil
}

func ApplyTokenByHandshake(_ctx context.Context, config model.ShellTimeConfig) (string, error) {
	ctx, span := commandTracer.Start(_ctx, "handshake", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	hs := model.NewHandshakeService(config)

	hid, err := hs.Init(ctx)
	if err != nil {
		return "", err
	}

	startedAt := time.Now()

	feLink := fmt.Sprintf("%s/cli/integration?hid=%s", config.WebEndpoint, hid)

	if err := browser.OpenURL(feLink); err != nil {
		slog.Error("failed to open browser", slog.Any("err", err))
	}

	color.Green.Println(fmt.Sprintf("Open %s to continue", feLink))

	s := spinner.New(spinner.CharSets[35], 200*time.Millisecond)
	s.Start()
	for {
		if time.Since(startedAt) > 10*time.Minute {
			color.Red.Println(" ❌ Failed to authenticate. Please retry with `shelltime init` or contact shelltime team (annatar.he+shelltime.xyz@gmail.com)")
			s.Stop()
			return "", fmt.Errorf("authentication timeout")
		}

		token, err := hs.Check(ctx, hid)
		if err != nil {
			return "", err
		}
		if token != "" {
			color.Green.Println(" ✅ You are ready to go!")
			s.Stop()
			return token, nil
		}

		time.Sleep(2 * time.Second)
	}
}

// generateSchemaFile generates the JSON schema file for config autocompletion
func generateSchemaFile(path string) error {
	reflector := &jsonschema.Reflector{
		AllowAdditionalProperties: false,
	}

	schema := reflector.Reflect(&model.ShellTimeConfig{})
	schema.Title = "ShellTime Configuration"
	schema.Description = "Configuration schema for shelltime CLI. Supports both YAML (.yaml, .yml) and TOML (.toml) formats."

	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	if err := os.WriteFile(path, schemaJSON, 0644); err != nil {
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	return nil
}
