package main

import (
	"fmt"
	"os"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/namespace"
	_ "github.com/YLonely/cer-manager/nsenter"
	"github.com/urfave/cli"
)

var nsexecCommand = cli.Command{
	Name:  "nsexec",
	Usage: "manage namespaces",
	Subcommands: []cli.Command{
		createCommand,
		releaseCommand,
		resetCommand,
	},
}

var createCommand = cli.Command{
	Name:      "create",
	Usage:     "create and initial a namespace",
	ArgsUsage: "NSTYPE {mnt|ipc|uts}",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "src",
			Usage: "specifiy the source(lower) dir of the overlay mount in new mount namespace",
		},
		cli.StringFlag{
			Name:  "bundle",
			Usage: "specifiy the path to the bundle if the type is mnt",
		},
	},
	Action: func(context *cli.Context) error {
		t := context.Args().First()
		if t == "" {
			printError("Namespace type must be provided\n")
			return nil
		}
		f := namespace.GetNamespaceFunction(namespace.NamespaceOpCreate, types.NamespaceType(t))
		if f != nil {
			err := f(context.String("src"), context.String("bundle"))
			if err != nil {
				printError("Failed to invoke namespace function %s\n", err.Error())
				return nil
			}
		}
		//we have to return our pid here
		fmt.Printf("ret:%d\n", os.Getpid())
		//and wait for parent process to open the namespace file
		var dummy string
		fmt.Scanln(&dummy)
		return nil
	},
}

var releaseCommand = cli.Command{
	Name:      "release",
	Usage:     "release the resources inside a namespace",
	ArgsUsage: "NSTYPE {mnt|ipc|uts}",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle",
			Usage: "spacifiy the path to the bundle if the ns type is mnt",
		},
	},
	Action: func(context *cli.Context) error {
		t := context.Args().First()
		if t == "" {
			printError("Namespace type must be provided\n")
			return nil
		}
		f := namespace.GetNamespaceFunction(namespace.NamespaceOpRelease, types.NamespaceType(t))
		if f != nil {
			err := f(context.String("bundle"))
			if err != nil {
				printError("Failed to invoke namespace function %s\n", err.Error())
				return nil
			}
		}
		fmt.Println("ret:OK")
		return nil
	},
}

var resetCommand = cli.Command{
	Name:      "reset",
	Usage:     "reset a already exists namespace for reuse",
	ArgsUsage: "NSTYPE {mnt|ipc|uts}",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "bundle",
			Usage: "spacifiy the path to the bundle if the ns type is mnt",
		},
	},
	Action: func(context *cli.Context) error {
		t := context.Args().First()
		if t == "" {
			printError("Namespace type must be provided\n")
			return nil
		}
		f := namespace.GetNamespaceFunction(namespace.NamespaceOpReset, types.NamespaceType(t))
		if f != nil {
			err := f(context.String("bundle"))
			if err != nil {
				printError("Failed to invoke namespace function %s\n", err.Error())
				return nil
			}
		}
		fmt.Println("ret:OK")
		return nil
	},
}

func printError(format string, a ...interface{}) {
	fmt.Printf("error:"+format, a...)
}
