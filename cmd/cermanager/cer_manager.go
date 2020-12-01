package main

import (
	"os"

	cerm "github.com/YLonely/cer-manager"
	"github.com/YLonely/cer-manager/log"
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
		log.Logger(cerm.MainService, "").WithError(err).Error()
	}
}
