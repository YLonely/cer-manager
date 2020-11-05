package main

import (
	_ "github.com/YLonely/cr-daemon/nsenter"
	"github.com/urfave/cli"
)

var nsexecCommand = cli.Command{
	Name:        "nsexec",
	Usage:       "manage namespaces",
	Subcommands: []cli.Command{},
}

var createCommand = cli.Command{
	Name:  "create",
	Usage: "create and initial a namespace",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:     "type",
			Usage:    "specifiy the type of the namespace",
			Required: true,
		},
		cli.StringFlag{
			Name:  "rootfs",
			Usage: "specifiy the path to the rootfs if the type is mnt",
		},
	},
}
