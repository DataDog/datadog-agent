// +build linux

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/system-probe/app"
)

func main() {
	setDefaultCommandIfNonePresent()
	checkForDeprecatedFlags()
	if err := app.SysprobeCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
