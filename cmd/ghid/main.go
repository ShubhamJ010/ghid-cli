package main

import (
	"os"

	"github.com/ghost-hid/ghid/internal/cli"
)

const Version = "1.0.0"

func main() {
	app := cli.NewApp(Version)
	os.Exit(app.Run(os.Args[1:]))
}
