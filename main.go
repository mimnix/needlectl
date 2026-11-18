package main

import (
	"os"

	"github.com/mimnix/needlectl/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
