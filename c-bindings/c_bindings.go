package main

import "os"
import "unsafe"
import "path/filepath"

import "github.com/syncthing/syncthing/lib/build"
import "github.com/syncthing/syncthing/lib/logger"
import "github.com/syncthing/syncthing/lib/locations"
import "github.com/syncthing/syncthing/lib/protocol"
import "github.com/syncthing/syncthing/lib/syncthing"

// include header for required C helper functions (so the following comment is NO comment)

// #include "c_bindings.h"
import "C"

var theApp *syncthing.App
var myID protocol.DeviceID

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

//export libst_run_syncthing
func libst_run_syncthing(config_dir string, gui_address string, gui_api_key string, verbose bool, allow_newer_config bool, no_default_config bool) int {
	// return if already running (for simplicity we only allow one Syncthing instance at at time for now)
	if theApp != nil {
		return 0
	}

	// set specified GUI address and API key
	if gui_address != "" {
		os.Setenv("STGUIADDRESS", gui_address)
	}
	if gui_api_key != "" {
		os.Setenv("STGUIADDRESS", gui_api_key)
	}

	// set specified config dir
	if config_dir != "" {
		if !filepath.IsAbs(config_dir) {
			var err error
			config_dir, err = filepath.Abs(config_dir)
			if err != nil {
				l.Warnln("Failed to make config path absolute:", err)
				return 3
			}
		}
		if err := locations.SetBaseDir(locations.ConfigBaseDir, config_dir); err != nil {
			l.Warnln(err)
			return 3
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

	// load config
	cfg, cfgErr := syncthing.LoadConfigAtStartup(locations.Get(locations.ConfigFile), cert, allow_newer_config, no_default_config)
	if cfgErr != nil {
		l.Warnln("Failed to initialize config:", cfgErr)
		return 2
	}

	// open database
	dbFile := locations.Get(locations.Database)
	ldb, dbErr := syncthing.OpenGoleveldb(dbFile)
	if dbErr != nil {
		l.Warnln("Error opening database:", dbErr)
		return 4
	}

	appOpts := syncthing.Options{
		AssetDir: os.Getenv("STGUIASSETS"),
		ProfilerURL: os.Getenv("STPROFILER"),
		NoUpgrade: true,
		Verbose: verbose,
	}
	theApp = syncthing.New(cfg, ldb, cert, appOpts)

	// start Syncthing and block until it has finished
	returnCode := int(theApp.Run())
	theApp = nil
	return returnCode
}

//export libst_stop_syncthing
func libst_stop_syncthing() int {
	if theApp != nil {
		return int(theApp.Stop(syncthing.ExitSuccess))
	} else {
		return 0;
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

func main() {
    // prevent "runtime.main_main·f: function main is undeclared in the main package"
}

