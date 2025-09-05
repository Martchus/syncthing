package main

import (
	"fmt"
	"os"
	syncthing_main "github.com/syncthing/syncthing/cmd/syncthing"
)

func main() {
	if err := syncthing_main.RunWithArgs(os.Args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
