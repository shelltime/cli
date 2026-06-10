package daemon

import (
	"time"

	"github.com/malamtime/cli/model"
)

var stConfig model.ConfigService
var version string
var startedAt time.Time

// commandStore is the daemon-owned bolt CommandStore, set when the bolt storage
// engine is enabled. It is nil when running with the default file engine.
var commandStore model.CommandStore

// InitCommandStore registers the daemon-owned command store used by the bolt
// track handlers. The daemon owns the store for its lifetime (bbolt holds an
// exclusive file lock).
func InitCommandStore(store model.CommandStore) {
	commandStore = store
}

const (
	PubSubTopic = "socket"
)

func Init(cs model.ConfigService, vs string) {
	stConfig = cs
	version = vs
	startedAt = time.Now()
}

func GetStartedAt() time.Time {
	return startedAt
}

func GetVersion() string {
	return version
}
