package panel

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/ipalpha-dev/tooling/internal/run"
)

// stopOnSignal stops every process when the terminal closes or the panel is killed, so nothing is
// left holding ports (the next ./run also cleans up what a hard crash leaves behind).
func stopOnSignal(ctl *run.Controller) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGTERM)
	go func() {
		<-ch
		ctl.StopAll()
		os.Exit(143)
	}()
}
