package main

import (
	"fmt"
	"os"
)

// Injected by GoReleaser via ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("supermodel %s (%s, %s)\n", version, commit, date)
			return
		}
	}
	fmt.Println("Supermodel CLI")
	fmt.Println("See https://supermodeltools.com for documentation.")
}
