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
			Name:  "src",
			Usage: "specifiy the source(lower) dir of the overlay mount in new mount namespace",
		},
		cli.StringFlag{
			Name:  "bundle",
			Usage: "specifiy the path to the bundle if the type is mnt",
		},
	},
}

var releaseCommand = cli.Command{
	Name:  "release",
	Usage: "release the resources inside a namespace",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:     "type",
			Usage:    "specifiy the type of the namespace",
			Required: true,
		},
		cli.IntFlag{
			Name:     "pid",
			Usage:    "specifiy the pid of the process which owns the namespace",
			Required: true,
		},
	},
}
