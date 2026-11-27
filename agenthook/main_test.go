package agenthook

import (
	"os"
	"testing"
)

// TestMain stubs OSUserLookup: an Antigravity sandbox runs as the OS user antigravity, which
// marks Antigravity events as cloud, and the suite must behave the same there as on CI. Tests of
// that check set the user themselves (asOSUser).
func TestMain(m *testing.M) {
	OSUserLookup = func() (string, error) { return "shelltime-test", nil }
	os.Exit(m.Run())
}
