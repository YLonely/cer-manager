package main

import (
	"context"
	"os"
	"os/signal"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/cermanager"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/signals"
	"github.com/urfave/cli"
)

var startCommand = cli.Command{
	Name:  "start",
	Usage: "start the manager",
	Action: func(c *cli.Context) error {
		signalC := make(chan os.Signal, 2048)
		ctx, cancel := context.WithCancel(context.Background())
		s, err := cermanager.NewServer()
		if err != nil {
			return err
		}
		errorC := s.Start(ctx)
		signal.Notify(signalC, signals.HandledSignals...)
		done := signals.HandleSignals(signalC, errorC)
		log.Logger(cerm.MainService, "").Info("Daemon started")
		<-done
		cancel()
		log.Logger(cerm.MainService, "").Info("Shutting down")
		s.Shutdown()
		return nil
	},
}
