package model

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellTimeConfigLogValue_HidesSecrets(t *testing.T) {
	const (
		token         = "st_v1_0123456789abcdefSECRETtokenBODY9876"
		endpointToken = "st_v1_endpointSECRETtoken0000000000000000"
		codeToken     = "st_v1_codetrackingSECRETtoken00000000000"
		proxyPassword = "proxySECRETpass"
		excludeSecret = "hunter2SECRET"
	)
	enabled := true
	cfg := ShellTimeConfig{
		Token:        token,
		APIEndpoint:  "https://api.shelltime.xyz",
		Endpoints:    []Endpoint{{APIEndpoint: "https://debug.example.com", Token: endpointToken}},
		Exclude:      []string{".*" + excludeSecret + ".*"},
		CodeTracking: &CodeTracking{Enabled: &enabled, Token: codeToken},
		AICodeOtel:   &AICodeOtel{Enabled: &enabled, GRPCPort: 54027},
		Proxy:        &ProxyConfig{URL: "socks5://user:" + proxyPassword + "@127.0.0.1:1080"},
		SocketPath:   "/tmp/shelltime.sock",
	}

	handlers := map[string]func(*bytes.Buffer) slog.Handler{
		"text": func(b *bytes.Buffer) slog.Handler {
			return slog.NewTextHandler(b, &slog.HandlerOptions{Level: slog.LevelDebug})
		},
		"json": func(b *bytes.Buffer) slog.Handler {
			return slog.NewJSONHandler(b, &slog.HandlerOptions{Level: slog.LevelDebug})
		},
	}
	for name, newHandler := range handlers {
		for form, value := range map[string]any{"value": cfg, "pointer": &cfg} {
			t.Run(name+"/"+form, func(t *testing.T) {
				var buf bytes.Buffer
				// The same call the daemon makes at startup.
				slog.New(newHandler(&buf)).DebugContext(context.Background(), "daemon.config", slog.Any("config", value))
				out := buf.String()

				for _, secret := range []string{token, endpointToken, codeToken, proxyPassword, excludeSecret} {
					assert.NotContains(t, out, secret)
				}
				assert.Contains(t, out, "st_v")
				assert.Contains(t, out, "9876")
				assert.Contains(t, out, "https://api.shelltime.xyz")
				assert.Contains(t, out, "https://debug.example.com")
				assert.Contains(t, out, "socks5://user:xxxxx@127.0.0.1:1080")
				assert.Contains(t, out, "54027")
				assert.Contains(t, out, "/tmp/shelltime.sock")
			})
		}
	}
}

func TestShellTimeConfigLogValue_EmptyConfig(t *testing.T) {
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("daemon.config", slog.Any("config", ShellTimeConfig{}))
	out := buf.String()

	assert.Contains(t, out, "config.token=\"\"")
	assert.Contains(t, out, "config.dataMasking=<nil>")
	assert.NotContains(t, out, "config.proxy")
}
