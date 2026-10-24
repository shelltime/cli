package daemon

import (
	"context"
	"sync/atomic"
	"time"
)

// ShutdownDrainTimeout bounds how long a stopping daemon waits for the messages it already
// acknowledged. launchd gives a job 20 seconds after SIGTERM before it kills it.
const ShutdownDrainTimeout = 5 * time.Second

// shutdownSignal is canceled when the daemon starts to stop. Editors drop the heartbeats
// the daemon acknowledged, so from then on queued heartbeats are saved for the resync
// service instead of sent, and a send in flight is cut short and saved too.
type shutdownSignal struct {
	ctx    context.Context
	cancel context.CancelFunc
}

var daemonShutdown atomic.Pointer[shutdownSignal]

func init() {
	resetShutdown()
}

func resetShutdown() {
	ctx, cancel := context.WithCancel(context.Background())
	daemonShutdown.Store(&shutdownSignal{ctx: ctx, cancel: cancel})
}

func beginShutdown() {
	daemonShutdown.Load().cancel()
}

func isShuttingDown() bool {
	return daemonShutdown.Load().ctx.Err() != nil
}

// withShutdown returns a context that is also canceled when the daemon starts to stop
func withShutdown(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(daemonShutdown.Load().ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}
