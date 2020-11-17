package main

import (
	"os"

	"github.com/YLonely/cer-manager/log"
	"github.com/YLonely/cer-manager/services"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "cer-manager"
	app.Usage = "cer-manager manages external resources for serverless container"
	app.Version = "v0.0.1"
	app.Commands = []cli.Command{
		startCommand,
		nsexecCommand,
	}

	if err := app.Run(os.Args); err != nil {
		log.Logger(services.MainService, "").WithError(err).Error()
	}
}
