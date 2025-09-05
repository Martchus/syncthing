// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package cli

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/willabides/kongplete"

	syncthing_main "github.com/syncthing/syncthing/cmd/syncthing"
	"github.com/syncthing/syncthing/lib/config"
)

type CLI struct {
	// repeat dir flags from "type CLI struct" in "cmd/syncthing/main.go" to have them despite only using the sub-level CLI parser
	ConfDir string `name:"config" short:"C" placeholder:"PATH" env:"STCONFDIR" help:"Set configuration directory (config and keys)"`
	DataDir string `name:"data" short:"D" placeholder:"PATH" env:"STDATADIR" help:"Set data directory (database and logs)"`
	HomeDir string `name:"home" short:"H" placeholder:"PATH" env:"STHOMEDIR" help:"Set configuration and data directory"`

	GUIAddress string `name:"gui-address" env:"STGUIADDRESS"`
	GUIAPIKey  string `name:"gui-apikey" env:"STGUIAPIKEY"`

	Show       showCommand      `cmd:"" help:"Show command group"`
	Debug      debugCommand     `cmd:"" help:"Debug command group"`
	Operations operationCommand `cmd:"" help:"Operation command group"`
	Errors     errorsCommand    `cmd:"" help:"Error command group"`
	Config     configCommand    `cmd:"" help:"Configuration modification command group" passthrough:""`
	Stdin      stdinCommand     `cmd:"" name:"-" help:"Read commands from stdin"`
}

type Context struct {
	clientFactory *apiClientFactory
}

func (cli CLI) AfterApply(kongCtx *kong.Context) error {
	error := syncthing_main.SetConfigDataLocationsFromFlags(cli.HomeDir, cli.ConfDir, cli.DataDir)
	if error != nil {
		return error
	}
	clientFactory := &apiClientFactory{
		cfg: config.GUIConfiguration{
			RawAddress: cli.GUIAddress,
			APIKey:     cli.GUIAPIKey,
		},
	}

	context := Context{
		clientFactory: clientFactory,
	}

	kongCtx.Bind(context)
	return nil
}

type stdinCommand struct{}

func RunWithArgs(args []string) error {
	var cli CLI
	p, err := kong.New(&cli)
	if err != nil {
		// can't happen, really
		return fmt.Errorf("creating parser: %w", err)
	}
	kongplete.Complete(p)
	ctx, err := p.Parse(args)
	if err != nil {
		fmt.Println("Error:", err)
		return err
	}
	if err := ctx.Run(); err != nil {
		fmt.Println("Error:", err)
		return err
	}
	return nil
}
