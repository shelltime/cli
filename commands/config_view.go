package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/trace"
)

var ConfigViewCommand *cli.Command = &cli.Command{
	Name:  "view",
	Usage: "view current configuration",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "format",
			Aliases: []string{"f"},
			Value:   "table",
			Usage:   "output format (table/json)",
		},
	},
	Action: configView,
	OnUsageError: func(cCtx *cli.Context, err error, isSubcommand bool) error {
		color.Red.Println(err.Error())
		return nil
	},
}

func configView(c *cli.Context) error {
	ctx, span := commandTracer.Start(c.Context, "config.view", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	SetupLogger(os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER))

	format := c.String("format")
	if format != "table" && format != "json" {
		return fmt.Errorf("unsupported format: %s. Use 'table' or 'json'", format)
	}

	cfg, err := configService.ReadConfigFile(ctx)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if format == "json" {
		return outputConfigJSON(cfg)
	}
	return outputConfigTable(cfg)
}

func outputConfigJSON(cfg model.ShellTimeConfig) error {
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

func outputConfigTable(cfg model.ShellTimeConfig) error {
	w := tablewriter.NewWriter(os.Stdout)
	w.Header([]string{"KEY", "VALUE"})

	pairs := flattenConfig(cfg, "")
	for _, pair := range pairs {
		w.Append([]string{pair.key, pair.value})
	}

	w.Render()
	return nil
}

type keyValuePair struct {
	key   string
	value string
}

func flattenConfig(v interface{}, prefix string) []keyValuePair {
	var pairs []keyValuePair

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return pairs
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return pairs
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Get JSON tag for the key name
		jsonTag := fieldType.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		// Parse the tag to get just the name part
		tagParts := strings.Split(jsonTag, ",")
		keyName := tagParts[0]

		fullKey := keyName
		if prefix != "" {
			fullKey = prefix + "." + keyName
		}

		// Handle pointer types
		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				pairs = append(pairs, keyValuePair{key: fullKey, value: "<not set>"})
				continue
			}
			field = field.Elem()
		}

		switch field.Kind() {
		case reflect.Struct:
			// Recursively flatten nested structs
			nestedPairs := flattenConfig(field.Interface(), fullKey)
			pairs = append(pairs, nestedPairs...)
		case reflect.Slice:
			if field.Len() == 0 {
				pairs = append(pairs, keyValuePair{key: fullKey, value: "[]"})
			} else {
				// Marshal slice to JSON for display
				jsonBytes, err := json.Marshal(field.Interface())
				if err != nil {
					pairs = append(pairs, keyValuePair{key: fullKey, value: fmt.Sprintf("<%d items>", field.Len())})
				} else {
					pairs = append(pairs, keyValuePair{key: fullKey, value: string(jsonBytes)})
				}
			}
		case reflect.Bool:
			pairs = append(pairs, keyValuePair{key: fullKey, value: fmt.Sprintf("%v", field.Bool())})
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			pairs = append(pairs, keyValuePair{key: fullKey, value: fmt.Sprintf("%d", field.Int())})
		case reflect.String:
			value := field.String()
			if value == "" {
				value = "<empty>"
			} else if strings.Contains(fullKey, "token") || strings.Contains(strings.ToLower(fullKey), "token") {
				// Mask sensitive fields
				if len(value) > 8 {
					value = value[:4] + "****" + value[len(value)-4:]
				} else {
					value = "****"
				}
			}
			pairs = append(pairs, keyValuePair{key: fullKey, value: value})
		default:
			pairs = append(pairs, keyValuePair{key: fullKey, value: fmt.Sprintf("%v", field.Interface())})
		}
	}

	return pairs
}
