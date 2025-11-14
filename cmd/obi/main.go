package main

import (
	"fmt"
	"os"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/app"
)

func main() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(app.Version())
		return
	}

	if err := app.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "obi: %v\n", err)
		os.Exit(1)
	}
}
