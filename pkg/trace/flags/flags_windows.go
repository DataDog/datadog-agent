// +build windows

package flags

import (
	"flag"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var DefaultConfigPath = "c:\\programdata\\datadog\\datadog.yaml"

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultConfigPath = filepath.Join(pd, "Datadog", "datadog.yaml")
	}
}
func registerOSSpecificFlags() {
	flag.BoolVar(&Win.InstallService, "install-service", false, "Install the trace agent to the Service Control Manager")
	flag.BoolVar(&Win.UninstallService, "uninstall-service", false, "Remove the trace agent from the Service Control Manager")
	flag.BoolVar(&Win.StartService, "start-service", false, "Starts the trace agent service")
	flag.BoolVar(&Win.StopService, "stop-service", false, "Stops the trace agent service")
}
