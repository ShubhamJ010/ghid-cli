package main

import (
	"os"

	"github.com/ShubhamJ010/ghid-cli/internal/cli"
)

const Version = "0.1.0"

func main() {
	app := cli.NewApp(Version)
	os.Exit(app.Run(os.Args[1:]))
}
