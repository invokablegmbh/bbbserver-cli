package main

import (
	"os"

	"bbbserver-cli/internal/cli"
)

func main() {
	app := cli.New()
	os.Exit(app.Execute())
}
