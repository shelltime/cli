package daemon

import (
	"time"

	"github.com/malamtime/cli/model"
)

var stConfig model.ConfigService
var version string
var startedAt time.Time

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
