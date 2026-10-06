package model

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"pretty name", "NAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n", "Ubuntu 24.04.1 LTS"},
		{"name and version", "NAME='Alpine Linux'\nVERSION_ID=3.20.2\n", "Alpine Linux 3.20.2"},
		{"comments ignored", "# PRETTY_NAME=nope\nNAME=Arch Linux\n", "Arch Linux"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseOSRelease(tt.content))
		})
	}
}

func TestParseProcUptime(t *testing.T) {
	secs, ok := parseProcUptime("350735.47 234388.90\n")
	assert.True(t, ok)
	assert.Equal(t, int64(350735), secs)

	_, ok = parseProcUptime("")
	assert.False(t, ok)
	_, ok = parseProcUptime("abc 1")
	assert.False(t, ok)
}

func TestParseProcLoadavg(t *testing.T) {
	loads, ok := parseProcLoadavg("0.52 0.58 0.59 1/389 12345\n")
	assert.True(t, ok)
	assert.Equal(t, []float64{0.52, 0.58, 0.59}, loads)

	_, ok = parseProcLoadavg("0.52 0.58")
	assert.False(t, ok)
	_, ok = parseProcLoadavg("x y z")
	assert.False(t, ok)
}

func TestParseDarwinLoadavg(t *testing.T) {
	raw := make([]byte, 24)
	const scale = 2048
	binary.LittleEndian.PutUint32(raw[0:], uint32(1.5*scale))
	binary.LittleEndian.PutUint32(raw[4:], uint32(0.75*scale))
	binary.LittleEndian.PutUint32(raw[8:], uint32(2.0*scale))
	binary.LittleEndian.PutUint64(raw[16:], scale)

	loads, ok := parseDarwinLoadavg(raw)
	assert.True(t, ok)
	assert.Equal(t, []float64{1.5, 0.75, 2}, loads)

	_, ok = parseDarwinLoadavg(raw[:16])
	assert.False(t, ok)
	_, ok = parseDarwinLoadavg(make([]byte, 24))
	assert.False(t, ok, "zero scale")
}

func TestReadSysStatDoesNotPanic(t *testing.T) {
	s := ReadSysStat()
	assert.GreaterOrEqual(t, s.UptimeSec, int64(0))
}
