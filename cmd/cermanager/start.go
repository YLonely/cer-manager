package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/YLonely/cer-manager/cermanager"
	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/signals"
	"github.com/urfave/cli"
)

var startCommand = cli.Command{
	Name:  "start",
	Usage: "start the manager",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "http-port",
			Usage: "enable the http server of cer-manager on [port]",
		},
	},
	Action: func(c *cli.Context) error {
		if c.GlobalBool("debug") {
			log.SetLevel(log.LevelDebug)
		} else {
			log.SetLevel(log.LevelInfo)
		}
		signalC := make(chan os.Signal, 2048)
		ctx, cancel := context.WithCancel(context.Background())
		s, err := cermanager.NewServer(c.Int("http-port"))
		if err != nil {
			cancel()
			return err
		}
		errorC := s.Start(ctx)
		signal.Notify(signalC, signals.HandledSignals...)
		done := signals.HandleSignals(signalC, errorC)
		log.Raw().Info("daemon started")
		<-done
		cancel()
		log.Raw().Info("shutting down")
		s.Shutdown()
		return nil
	},
}
