package main

import (
	"fmt"

	"github.com/YLonely/cer-manager/api/types"
	"github.com/YLonely/cer-manager/namespace"
	_ "github.com/YLonely/cer-manager/nsenter"
	"github.com/urfave/cli"
)

var nsexecCommand = cli.Command{
	Name:      "nsexec",
	Usage:     "execute functions in a namespace",
	ArgsUsage: "FUNCTION_KEY NSTYPE {mnt|ipc|uts}",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "src",
			Usage: "specifiy the source(lower) dir of the overlay mount in new mount namespace",
		},
		cli.StringFlag{
			Name:  "bundle",
			Usage: "specifiy the path to the bundle if the type is mnt",
		},
		cli.StringFlag{
			Name:  "checkpoint",
			Usage: "specifiy the path to the checkpoint files if the type is mnt",
		},
	},
	Action: func(context *cli.Context) error {
		key := context.Args().First()
		nsType := context.Args().Get(1)
		var ret []byte
		var err error
		if nsType == "" {
			printError("namespace type must be provided")
			return nil
		}
		f := namespace.GetNamespaceFunction(namespace.NamespaceFunctionKey(key), types.NamespaceType(nsType))
		if f != nil {
			ret, err = f(
				map[string]interface{}{
					"src":        context.String("src"),
					"bundle":     context.String("bundle"),
					"checkpoint": context.String("checkpoint"),
				},
			)
			if err != nil {
				printError("namespace function returns error %s", err.Error())
				return nil
			}
		}
		fmt.Print("ret:" + withSizePrefixed(string(ret)))
		// wait for the parent to release me
		var dummy string
		fmt.Scanln(&dummy)
		return nil
	},
}

func printError(format string, a ...interface{}) {
	str := fmt.Sprintf(format, a...)
	fmt.Print("err:" + withSizePrefixed(str))
}

func withSizePrefixed(data string) string {
	n := len(data)
	return fmt.Sprintf("%d,%s", n, data)
}
