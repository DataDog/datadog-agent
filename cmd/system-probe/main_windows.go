// +build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/system-probe/app"
	"github.com/DataDog/datadog-agent/cmd/system-probe/windows/service"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows/svc"
)

var (
	defaultSysProbeConfigPath = "c:\\programdata\\datadog\\system-probe.yaml"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultSysProbeConfigPath = filepath.Join(pd, "system-probe.yaml")
	}
}

func main() {
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("Failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			service.RunService(false)
			return
		}
	}

	setDefaultCommandIfNonePresent()
	checkForDeprecatedFlags()
	if err := app.SysprobeCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
