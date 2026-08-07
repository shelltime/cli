package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewReleaseSource(t *testing.T) {
	t.Run("http endpoint becomes a proxy", func(t *testing.T) {
		s := NewReleaseSource("https://api.shelltime.xyz")
		assert.True(t, s.IsProxy())
		assert.Equal(t, "https://api.shelltime.xyz", s.ProxyBase)
	})

	t.Run("trailing slash is trimmed", func(t *testing.T) {
		s := NewReleaseSource("https://api.shelltime.xyz/")
		assert.Equal(t, "https://api.shelltime.xyz", s.ProxyBase)
	})

	t.Run("empty or non-http falls back to GitHub direct", func(t *testing.T) {
		for _, in := range []string{"", "  ", "api.shelltime.xyz", "ftp://x"} {
			s := NewReleaseSource(in)
			assert.False(t, s.IsProxy(), "input %q should not produce a proxy", in)
		}
	})
}

func TestReleaseSource_URLs(t *testing.T) {
	t.Run("zero value points at GitHub", func(t *testing.T) {
		var s ReleaseSource
		assert.Equal(t, BuildDownloadURL("v1.2.3", "cli_Darwin_arm64.zip"),
			s.DownloadURL("v1.2.3", "cli_Darwin_arm64.zip"))
		assert.Equal(t, BuildChecksumsURL("v1.2.3"), s.ChecksumsURL("v1.2.3"))
	})

	t.Run("proxy points at the API", func(t *testing.T) {
		s := NewReleaseSource("https://api.shelltime.xyz")
		assert.Equal(t,
			"https://api.shelltime.xyz/api/v1/cli/releases/v1.2.3/cli_Darwin_arm64.zip",
			s.DownloadURL("v1.2.3", "cli_Darwin_arm64.zip"))
		assert.Equal(t,
			"https://api.shelltime.xyz/api/v1/cli/releases/v1.2.3/checksums.txt",
			s.ChecksumsURL("v1.2.3"))
	})

	t.Run("Direct escapes the proxy", func(t *testing.T) {
		s := NewReleaseSource("https://api.shelltime.xyz")
		assert.False(t, s.Direct().IsProxy())
		assert.Equal(t, BuildDownloadURL("v1.2.3", "a.zip"), s.Direct().DownloadURL("v1.2.3", "a.zip"))
	})
}
