// Command xdl-app is the XDL desktop app (XDL.app). Given a command, as
// `XDL.app/Contents/MacOS/XDL download <user>`, it runs the CLI instead, so
// the app's executable doubles as `xdl`.
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"

	"github.com/missuo/xdl/internal/cli"
	"github.com/missuo/xdl/internal/gui"
	"github.com/missuo/xdl/internal/version"
)

func main() {
	// mygo build links the version of mygo.json into the app.
	if v := mygo.App.Version(); v != "" {
		version.Version = v
	}
	if len(os.Args) > 1 && cli.IsCommand(os.Args[1]) {
		os.Exit(cli.Main(os.Args[1:]))
	}
	if err := gui.Run(); err != nil {
		log.Fatal(err)
	}
}
