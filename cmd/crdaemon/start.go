package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/YLonely/cr-daemon/crdaemon"
	"github.com/YLonely/cr-daemon/log"
	"github.com/YLonely/cr-daemon/service"
	"github.com/YLonely/cr-daemon/signals"
	"github.com/urfave/cli"
)

var startCommand = cli.Command{
	Name:  "start",
	Usage: "start the crdaemon",
	Action: func(c *cli.Context) error {
		signalC := make(chan os.Signal, 2048)
		ctx, cancel := context.WithCancel(context.Background())
		s, err := crdaemon.NewServer()
		if err != nil {
			return err
		}
		errorC := s.Start(ctx)
		signal.Notify(signalC, signals.HandledSignals...)
		done := signals.HandleSignals(signalC, errorC)
		log.Logger(service.MainService, "").Info("Daemon started")
		<-done
		cancel()
		log.Logger(service.MainService, "").Info("Shutting down")
		s.Shutdown()
		return nil
	},
}
