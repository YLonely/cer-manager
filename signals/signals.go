package signals

import (
	"os"
	"syscall"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/log"
)

var HandledSignals = []os.Signal{
	syscall.SIGTERM,
	syscall.SIGINT,
}

func HandleSignals(signals chan os.Signal, errorC chan error) chan struct{} {
	done := make(chan struct{}, 1)
	go func() {
		select {
		case <-signals:
			log.Logger(cerm.MainService, "").Info("Receive a signal")
		case err := <-errorC:
			log.Logger(cerm.MainService, "").WithError(err).Error()
		}
		close(done)
	}()
	return done
}
