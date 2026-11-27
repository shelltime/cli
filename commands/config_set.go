package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/gookit/color"
	"github.com/malamtime/cli/model"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
)

// configSwitch is a boolean setting that `shelltime config enable|disable` changes.
type configSwitch struct {
	key   string                           // as written in the config file, e.g. ai.agent.view
	value func(model.ShellTimeConfig) bool // the setting as the merged config has it
}

var configSwitches = []configSwitch{
	{"ai.agent.view", func(c model.ShellTimeConfig) bool { return c.AI != nil && c.AI.Agent.View }},
	{"ai.agent.edit", func(c model.ShellTimeConfig) bool { return c.AI != nil && c.AI.Agent.Edit }},
	{"ai.agent.delete", func(c model.ShellTimeConfig) bool { return c.AI != nil && c.AI.Agent.Delete }},
	{"ai.showTips", shouldShowTips},
	{"ai.shareContext", func(c model.ShellTimeConfig) bool { return shareContextEnabled(c.AI) }},
	{"agentHooks.sendContent", func(c model.ShellTimeConfig) bool { return c.AgentHooksSendContent() }},
}

const configSwitchDescription = `Settings:
  ai.agent.view       shelltime q runs suggested read-only commands (ls, cat, grep)
  ai.agent.edit       shelltime q runs suggested commands that change files
  ai.agent.delete     shelltime q runs suggested delete commands once you confirm
  ai.showTips         shelltime q shows tips
  ai.shareContext     shelltime q sends context about where you run it
  agentHooks.sendContent
                      the Cursor and Antigravity hooks send prompt, response and
                      tool input text (off: only the activity)

Name several settings of one section after it, separated by commas or spaces.
The change goes to config.local.* when that file has the setting's section
(it overrides config.yaml's), else to config.yaml. Only the setting's lines
change, so comments and layout stay; for unusual YAML (a {...} mapping, a quoted
value) the file is written again, which keeps comments but drops blank lines.`

var ConfigEnableCommand *cli.Command = &cli.Command{
	Name:      "enable",
	Usage:     "turn a setting on",
	ArgsUsage: "<setting> [names]",
	Description: configSwitchDescription + `

Examples:
  shelltime config enable ai.agent view,edit
  shelltime config enable ai.agent.view
  shelltime config enable ai.showTips`,
	Action: func(c *cli.Context) error { return configSetSwitches(c, true) },
}

var ConfigDisableCommand *cli.Command = &cli.Command{
	Name:      "disable",
	Usage:     "turn a setting off",
	ArgsUsage: "<setting> [names]",
	Description: configSwitchDescription + `

Examples:
  shelltime config disable ai.agent delete
  shelltime config disable ai.agent view,edit,delete
  shelltime config disable ai.showTips
  shelltime config disable agentHooks.sendContent`,
	Action: func(c *cli.Context) error { return configSetSwitches(c, false) },
}

func configSetSwitches(c *cli.Context, on bool) error {
	ctx, span := commandTracer.Start(c.Context, "config.set")
	defer span.End()

	configDir := os.ExpandEnv("$HOME/" + model.COMMAND_BASE_STORAGE_FOLDER)
	SetupLogger(configDir)

	switches, err := resolveConfigSwitches(c.Args().Slice())
	if err != nil {
		msg := err.Error()
		color.Red.Println(strings.ToUpper(msg[:1]) + msg[1:])
		return err
	}
	keys := make([]string, len(switches))
	settings := make([]model.ConfigBool, len(switches))
	for i, sw := range switches {
		keys[i] = sw.key
		settings[i] = model.ConfigBool{Path: strings.Split(sw.key, "."), Value: on}
	}
	span.SetAttributes(attribute.StringSlice("config.keys", keys), attribute.Bool("config.value", on))

	files, err := model.WriteConfigBools(configDir, settings)
	if err != nil {
		if errors.Is(err, model.ErrConfigNotFound) {
			color.Red.Println("No config file yet. Run `shelltime init` first.")
		} else {
			color.Red.Println(err.Error())
		}
		return err
	}

	cfg, err := configService.ReadConfigFile(ctx, model.WithSkipCache())
	if err != nil {
		color.Red.Printf("Saved, but the config no longer loads: %v\n", err)
		return err
	}

	state := map[bool]string{true: "on", false: "off"}
	for _, sw := range switches {
		if sw.value(cfg) == on {
			color.Green.Printf("✅ %s is %s\n", sw.key, state[on])
		} else {
			// The file says so, but something else decides (a value the merge doesn't read)
			color.Yellow.Printf("⚠️  %s is still %s: check it with `shelltime config view`\n", sw.key, state[!on])
		}
	}
	fmt.Println(color.Gray.Sprintf("Saved to %s", strings.Join(homeRelative(files), ", ")))

	if on && slices.Contains(keys, "ai.agent.delete") {
		color.Yellow.Println("shelltime q still asks before it runs a delete command.")
	}
	return nil
}

// resolveConfigSwitches reads `<setting> [names]`: ai.agent.view, or ai.agent view,edit.
func resolveConfigSwitches(args []string) ([]configSwitch, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("name a setting, e.g. `ai.agent view,edit`. Settings: %s", configSwitchKeys())
	}
	names := []string{args[0]}
	if len(args) > 1 {
		if sw, ok := findConfigSwitch(args[0]); ok {
			section := sw.key[:strings.LastIndex(sw.key, ".")]
			under := configSwitchesUnder(section)
			return nil, fmt.Errorf("%s is a setting, not a section: name it alone, or the section first, e.g. `%s %s`",
				sw.key, section, strings.Join(under[:min(2, len(under))], ","))
		}
		names = nil
		for _, arg := range args[1:] {
			for _, name := range strings.FieldsFunc(arg, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
				names = append(names, args[0]+"."+name)
			}
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("name the settings of %s to change", args[0])
		}
	}

	var switches []configSwitch
	for _, name := range names {
		sw, ok := findConfigSwitch(name)
		if !ok {
			if under := configSwitchesUnder(name); len(under) > 0 {
				return nil, fmt.Errorf("name the settings of %s to change: %s, e.g. `%s %s`",
					name, strings.Join(under, ", "), name, strings.Join(under[:min(2, len(under))], ","))
			}
			return nil, fmt.Errorf("unknown setting %q. Settings: %s", name, configSwitchKeys())
		}
		if !slices.ContainsFunc(switches, func(s configSwitch) bool { return s.key == sw.key }) {
			switches = append(switches, sw)
		}
	}
	return switches, nil
}

func findConfigSwitch(name string) (configSwitch, bool) {
	for _, sw := range configSwitches {
		if strings.EqualFold(sw.key, name) {
			return sw, true
		}
	}
	return configSwitch{}, false
}

// configSwitchesUnder returns the names of the settings directly in section, e.g. view, edit
// and delete for ai.agent.
func configSwitchesUnder(section string) []string {
	var names []string
	for _, sw := range configSwitches {
		dot := strings.LastIndex(sw.key, ".")
		if strings.EqualFold(sw.key[:dot], section) {
			names = append(names, sw.key[dot+1:])
		}
	}
	return names
}

func configSwitchKeys() string {
	keys := make([]string, len(configSwitches))
	for i, sw := range configSwitches {
		keys[i] = sw.key
	}
	return strings.Join(keys, ", ")
}

func homeRelative(paths []string) []string {
	home, err := os.UserHomeDir()
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = p
		if rel, relErr := filepath.Rel(home, p); err == nil && relErr == nil && !strings.HasPrefix(rel, "..") {
			out[i] = filepath.Join("~", rel)
		}
	}
	return out
}
