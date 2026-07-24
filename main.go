package main

import (
	"os"

	"github.com/wildcard/gh-code-review/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
