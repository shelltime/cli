package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gookit/color"
	"github.com/invopop/jsonschema"
	"github.com/malamtime/cli/model"
	"github.com/malamtime/cli/stloader"
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
		emailFlag,
	},
	Action: commandAuth,
}

// emailFlag signs in by approving a request in the ShellTime iOS app instead
// of the browser. It needs no browser, so agents can run it.
var emailFlag = &cli.StringFlag{
	Name:     "email",
	Aliases:  []string{"e"},
	Usage:    "Sign in by approving a request on the iPhone of the account with this email (ShellTime iOS app)",
	Required: false,
}

// ErrAuthNotApproved means signing in through the iOS app did not complete:
// the request was denied, expired or refused. The CLI exits non-zero.
var ErrAuthNotApproved = errors.New("sign-in was not approved")

// newAppApprovalService and appApprovalSleep are replaced in tests.
var (
	newAppApprovalService = model.NewAppApprovalService
	appApprovalSleep      = sleepContext
)

func commandAuth(c *cli.Context) error {
	newToken := c.String("token")
	email := c.String("email")
	if newToken != "" && email != "" {
		return errors.New("use either --token or --email, not both")
	}

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

	if newToken == "" && email != "" {
		nt, err := ApplyTokenByAppApproval(ctx, config, email)
		if err != nil {
			return err
		}
		newToken = nt
	} else if newToken == "" {
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

	l := stloader.NewLoader(stloader.LoaderConfig{
		Text:          "Waiting for authentication...",
		EnableShining: true,
		BaseColor:     stloader.RGB{R: 100, G: 180, B: 255},
	})
	l.Start()
	for {
		if time.Since(startedAt) > 10*time.Minute {
			l.Stop()
			color.Red.Println(" ❌ Failed to authenticate. Please retry with `shelltime init` or contact shelltime team (annatar.he+shelltime.xyz@gmail.com)")
			return "", fmt.Errorf("authentication timeout")
		}

		token, err := hs.Check(ctx, hid)
		if err != nil {
			l.Stop()
			return "", err
		}
		if token != "" {
			l.Stop()
			color.Green.Println(" ✅ You are ready to go!")
			return token, nil
		}

		time.Sleep(2 * time.Second)
	}
}

// ApplyTokenByAppApproval pushes a sign-in request to the ShellTime iOS app of
// the account with this email and waits for the user to approve it. Output is
// plain lines, so an agent driving the CLI can relay the code to the user.
func ApplyTokenByAppApproval(_ctx context.Context, config model.ShellTimeConfig, email string) (string, error) {
	ctx, span := commandTracer.Start(_ctx, "appApproval", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	svc := newAppApprovalService(config)
	ticket, err := svc.Request(ctx, email)
	if err != nil {
		var apiErr *model.AppApprovalError
		if errors.As(err, &apiErr) {
			color.Red.Println(" ❌ " + apiErr.Message)
			if apiErr.StatusCode == http.StatusNotFound {
				color.Yellow.Println("Run `shelltime auth` without --email to sign in with your browser instead.")
			}
		} else {
			color.Red.Println(fmt.Sprintf(" ❌ Failed to send the sign-in request: %v", err))
		}
		return "", fmt.Errorf("%w: %w", ErrAuthNotApproved, err)
	}

	printAppApprovalIntro(os.Stdout, ticket)
	stopIndicator := startWaitingIndicator()
	token, err := waitForAppApproval(ctx, svc, ticket, appApprovalSleep)
	stopIndicator()
	if err != nil {
		color.Red.Println(" ❌ " + err.Error())
		return "", err
	}

	color.Green.Println(" ✅ You are ready to go!")
	return token, nil
}

func printAppApprovalIntro(out io.Writer, ticket model.AppApprovalTicket) {
	fmt.Fprintln(out, "Sent a sign-in request to the ShellTime app on your iPhone.")
	fmt.Fprintf(out, "Code: %s. Check that your iPhone shows the same code, then approve it.\n", ticket.MatchCode)
	fmt.Fprintf(out, "Waiting for approval (expires in %s)...\n", time.Duration(ticket.ExpiresIn)*time.Second)
}

// appApprovalPollGrace keeps polling a little past the expiry in case the
// server's clock is ahead; the server answers "expired" on its own anyway.
const appApprovalPollGrace = 30 * time.Second

// waitForAppApproval polls until the request is approved, denied or expired.
// Network and server errors are retried until the request expires; a request
// the API refuses ends the wait.
func waitForAppApproval(
	ctx context.Context,
	svc model.AppApprovalService,
	ticket model.AppApprovalTicket,
	sleep func(context.Context, time.Duration) error,
) (string, error) {
	// The server sends 2 s; keep a bad value from stalling or hammering it
	interval := time.Duration(min(max(ticket.Interval, 1), 30)) * time.Second
	giveUpAt := time.Now().Add(time.Duration(ticket.ExpiresIn)*time.Second + appApprovalPollGrace)

	for {
		if err := sleep(ctx, interval); err != nil {
			return "", err
		}

		result, err := svc.Poll(ctx, ticket.RequestID, ticket.PollToken)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			var apiErr *model.AppApprovalError
			if errors.As(err, &apiErr) && !retryableAppApprovalStatus(apiErr.StatusCode) {
				return "", fmt.Errorf("%w: %s", ErrAuthNotApproved, apiErr.Message)
			}
			slog.Warn("app approval poll failed, retrying", slog.Any("err", err))
		} else {
			switch result.Status {
			case model.AppApprovalApproved:
				if result.OpenToken == nil || result.OpenToken.Token == "" {
					return "", fmt.Errorf("%w: the server approved the request but sent no token", ErrAuthNotApproved)
				}
				return result.OpenToken.Token, nil
			case model.AppApprovalDenied:
				return "", fmt.Errorf("%w: the request was denied on your iPhone", ErrAuthNotApproved)
			case model.AppApprovalExpired:
				return "", fmt.Errorf("%w: the request expired before it was approved", ErrAuthNotApproved)
			}
		}

		if time.Now().After(giveUpAt) {
			return "", fmt.Errorf("%w: the request expired before it was approved", ErrAuthNotApproved)
		}
	}
}

// retryableAppApprovalStatus: a poll that failed with this status may succeed
// next time (a server error, rate limiting, or a proxy timing out).
func retryableAppApprovalStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return true
	default:
		return status >= http.StatusInternalServerError
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// startWaitingIndicator shows a spinner when a person is watching the
// terminal; piped output (an agent, a script) stays plain. It returns the
// function that stops it.
func startWaitingIndicator() func() {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return func() {}
	}
	l := stloader.NewLoader(stloader.LoaderConfig{
		Text:          "Waiting for approval on your iPhone...",
		EnableShining: true,
		BaseColor:     stloader.RGB{R: 100, G: 180, B: 255},
	})
	l.Start()
	return l.Stop
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
