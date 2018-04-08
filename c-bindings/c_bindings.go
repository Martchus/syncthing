package main

import (
	"bytes"
	"context"
	"fmt"
	_ "net/http/pprof" // Need to import this to support STPROFILER.
	"os"
	"path/filepath"
	"time"
	"unsafe"

	syncthing_main "github.com/syncthing/syncthing/cmd/syncthing"
	"github.com/syncthing/syncthing/cmd/syncthing/cli"
	"github.com/syncthing/syncthing/lib/build"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/logger"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/signature"
	"github.com/syncthing/syncthing/lib/syncthing"
	"github.com/thejerf/suture/v4"
)

// include header for required C helper functions (so the following comment is NO comment)

// #include "c_bindings.h"
import "C"

var theApp *syncthing.App
var myID protocol.DeviceID
var cliArgs []string

const (
	tlsDefaultCommonName = "syncthing"
)

//export libst_own_device_id
func libst_own_device_id() string {
	return myID.String()
}

//export libst_init_logging
func libst_init_logging() {
	l.AddHandler(logger.LevelVerbose, func(level logger.LogLevel, msg string) {
		runes := []byte(msg)
		length := len(runes)
		if length <= 0 {
			return
		}
		C.libst_invoke_logging_callback(C.int(level), (*C.char)(unsafe.Pointer(&runes[0])), C.size_t(len(runes)))
	})
}

//export libst_clear_cli_args
func libst_clear_cli_args(command string) {
	if command == "cli" {
		cliArgs = []string{}
	} else {
		cliArgs = []string{command}
	}
}

//export libst_append_cli_arg
func libst_append_cli_arg(arg string) {
	cliArgs = append(cliArgs, arg)
}

//export libst_run_cli
func libst_run_cli() int {
	if err := cli.RunWithArgs(cliArgs); err != nil {
		fmt.Println(err)
		return 1
	}
	return 0
}

//export libst_run_main
func libst_run_main() int {
	if err := syncthing_main.RunWithArgs(cliArgs); err != nil {
		fmt.Println(err)
		return 1
	}
	return 0
}

//export libst_run_syncthing
func libst_run_syncthing(configDir string, dataDir string, guiAddress string, guiApiKey string, verbose bool, allowNewerConfig bool, noDefaultConfig bool, skipPortProbing bool, ensureConfigDirExists bool, ensureDataDirExists bool, expandPathsFromEnv bool) int {
	// return if already running (for simplicity we only allow one Syncthing instance at at time for now)
	if theApp != nil {
		return 0
	}

	// set specified GUI address and API key
	if guiAddress != "" {
		os.Setenv("STGUIADDRESS", guiAddress)
	}
	if guiApiKey != "" {
		os.Setenv("STGUIAPIKEY", guiApiKey)
	}

	// set specified config dir
	if configDir != "" {
		if expandPathsFromEnv {
			configDir = os.ExpandEnv(configDir)
		}
		if !filepath.IsAbs(configDir) {
			var err error
			configDir, err = filepath.Abs(configDir)
			if err != nil {
				l.Warnln("Failed to make config path absolute:", err)
				return 3
			}
		}
		if err := locations.SetBaseDir(locations.ConfigBaseDir, configDir); err != nil {
			l.Warnln(err)
			return 3
		}
	}

	// set specified database dir
	if dataDir != "" {
		if expandPathsFromEnv {
			dataDir = os.ExpandEnv(dataDir)
		}
		if !filepath.IsAbs(dataDir) {
			var err error
			dataDir, err = filepath.Abs(dataDir)
			if err != nil {
				l.Warnln("Failed to make database path absolute:", err)
				return 3
			}
		}
		if err := locations.SetBaseDir(locations.DataBaseDir, dataDir); err != nil {
			l.Warnln(err)
			return 3
		}
	}

	// ensure that the config directory exists
	if ensureConfigDirExists {
		if err := syncthing.EnsureDir(locations.GetBaseDir(locations.ConfigBaseDir), 0700); err != nil {
			l.Warnln("Failed to create config directory:", err)
			return 4
		}
	}

	// ensure that the database directory exists
	if dataDir != "" && ensureDataDirExists {
		if err := syncthing.EnsureDir(locations.GetBaseDir(locations.DataBaseDir), 0700); err != nil {
			l.Warnln("Failed to create database directory:", err)
			return 4
		}
	}

	// ensure that we have a certificate and key
	cert, certErr := syncthing.LoadOrGenerateCertificate(
		locations.Get(locations.CertFile),
		locations.Get(locations.KeyFile),
	)
	if certErr != nil {
		l.Warnln("Failed to load/generate certificate:", certErr)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// earlyService is a supervisor that runs the services needed for or
	// before app startup; the event logger, and the config service.
	spec := svcutil.SpecWithDebugLogger(l)
	earlyService := suture.New("early", spec)
	earlyService.ServeBackground(ctx)

	evLogger := events.NewLogger()
	earlyService.Add(evLogger)

	// load config
	configLocation := locations.Get(locations.ConfigFile)
	l.Infoln("Loading config from:", configLocation)
	cfgWrapper, cfgErr := syncthing.LoadConfigAtStartup(configLocation, cert, evLogger, allowNewerConfig, noDefaultConfig, skipPortProbing)
	if cfgErr != nil {
		l.Warnln("Failed to initialize config:", cfgErr)
		return 2
	}
	if cfgService, ok := cfgWrapper.(suture.Service); ok {
		earlyService.Add(cfgService)
	}

	// migrate legacy database
	dbDeleteRetentionInterval, _ := time.ParseDuration("4320h") // make this configurable
	if err := syncthing.TryMigrateDatabase(dbDeleteRetentionInterval); err != nil {
		l.Warnln("Failed to migrate old-style database:", err)
		return 4
	}

	// open database
	var err error
	sdb, err := syncthing.OpenDatabase(locations.Get(locations.Database), dbDeleteRetentionInterval)
	if err != nil {
		l.Warnln("Error opening database:", err)
		return 4
	}

	appOpts := syncthing.Options{
		ProfilerAddr: os.Getenv("STPROFILER"),
		NoUpgrade:    true,
		Verbose:      verbose,
	}
	theApp, err = syncthing.New(cfgWrapper, sdb, evLogger, cert, appOpts)
	if err != nil {
		l.Warnln("Failed to start Syncthing:", err)
		return svcutil.ExitError.AsInt()
	}

	// start Syncthing and block until it has finished
	returnCode := 0
	if err := theApp.Start(); err != nil {
		returnCode = svcutil.ExitError.AsInt()
	}
	returnCode = theApp.Wait().AsInt()
	theApp = nil
	return returnCode
}

//export libst_stop_syncthing
func libst_stop_syncthing() int {
	if theApp != nil {
		return int(theApp.Stop(svcutil.ExitSuccess))
	} else {
		return 0
	}
}

//export libst_reset_database
func libst_reset_database() {
	os.RemoveAll(locations.Get(locations.Database))
}

//export libst_syncthing_version
func libst_syncthing_version() *C.char {
	return C.CString(build.Version)
}

//export libst_long_syncthing_version
func libst_long_syncthing_version() *C.char {
	return C.CString(build.LongVersion)
}

//export libst_verify
func libst_verify(pubKeyPEM []byte, sig []byte, data []byte) *C.char {
	reader := bytes.NewReader(data)
	err := signature.Verify(pubKeyPEM, sig, reader)
	if err != nil {
		return C.CString(err.Error())
	}
	return C.CString("")
}

func main() {
	// prevent "runtime.main_main·f: function main is undeclared in the main package"
}
