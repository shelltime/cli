package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSSHSession(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "local shell", env: map[string]string{"TERM_PROGRAM": "ghostty"}, want: false},
		{name: "ssh connection", env: map[string]string{"SSH_CONNECTION": "10.0.0.2 51234 10.0.0.9 22"}, want: true},
		{name: "ssh client only", env: map[string]string{"SSH_CLIENT": "10.0.0.2 51234 22"}, want: true},
		{name: "ssh tty only", env: map[string]string{"SSH_TTY": "/dev/pts/1"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsSSHSession(func(k string) string { return tt.env[k] }))
		})
	}
}
