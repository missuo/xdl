// Command xdl is XDL's command line interface: it downloads X/Twitter media
// timelines, alone or through the running XDL app's queue.
package main

import (
	"os"

	"github.com/missuo/xdl/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
