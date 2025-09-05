package main

import (
	"fmt"
	"os"
	"github.com/syncthing/syncthing/cmd/syncthing/cli"
)

func main() {
	if err := cli.RunWithArgs(os.Args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
