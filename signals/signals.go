package signals

import (
	"os"
	"syscall"

	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/services"
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
			log.Logger(services.MainService, "").Info("Receive a signal")
		case err := <-errorC:
			log.Logger(services.MainService, "").WithError(err).Error()
		}
		close(done)
	}()
	return done
}