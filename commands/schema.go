package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/invopop/jsonschema"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
)

var SchemaCommand = &cli.Command{
	Name:  "schema",
	Usage: "Generate JSON schema for config file (for IDE autocompletion)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Output file path (default: stdout)",
		},
	},
	Action: func(c *cli.Context) error {
		reflector := &jsonschema.Reflector{
			AllowAdditionalProperties: false,
		}

		schema := reflector.Reflect(&model.ShellTimeConfig{})
		schema.Title = "ShellTime Configuration"
		schema.Description = "Configuration schema for shelltime CLI. Supports both YAML (.yaml, .yml) and TOML (.toml) formats."

		schemaJSON, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to generate schema: %w", err)
		}

		outputPath := c.String("output")
		if outputPath != "" {
			if err := os.WriteFile(outputPath, schemaJSON, 0644); err != nil {
				return fmt.Errorf("failed to write schema file: %w", err)
			}
			fmt.Printf("Schema written to %s\n", outputPath)
		} else {
			fmt.Println(string(schemaJSON))
		}

		return nil
	},
}
