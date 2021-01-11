package signals

import (
	"os"
	"syscall"

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
		case s := <-signals:
			log.Raw().Infof("receive a signal %v", s)
		case err := <-errorC:
			log.Raw().Error(err)
		}
		close(done)
	}()
	return done
}
