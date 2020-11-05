package main

import (
	"os"

	"github.com/YLonely/cr-daemon/log"
	"github.com/YLonely/cr-daemon/service"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "crdaemon"
	app.Usage = "crdaemon provides extra features for serverless container"
	app.Version = "v0.0.1"

	if err := app.Run(os.Args); err != nil {
		log.Logger(service.MainService, "").WithError(err).Error()
	}
}
