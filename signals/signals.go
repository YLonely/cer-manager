package signals

import (
	"os"
	"syscall"

	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/service"
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
			log.Logger(service.MainService, "").Info("Receive a signal")
		case err := <-errorC:
			log.Logger(service.MainService, "").WithError(err).Error()
		}
		close(done)
	}()
	return done
}
