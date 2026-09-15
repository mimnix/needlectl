package main

import (
	"os"

	"github.com/mimmo/needlectl/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
