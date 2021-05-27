//+build windows

package config

import (
	"fmt"
	"net"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// defaultSystemProbeAddress is the default address to be used for connecting to the system probe
	defaultSystemProbeAddress = "localhost:3333"

	// ServiceName is the service name used for the system-probe
	ServiceName = "datadog-system-probe"
)

var (
	defaultConfigDir              = "c:\\programdata\\datadog\\"
	defaultSystemProbeLogFilePath = "c:\\programdata\\datadog\\logs\\system-probe.log"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfigDir = pd
		defaultSystemProbeLogFilePath = filepath.Join(pd, "logs", "system-probe.log")
	}
}

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockAddress string) error {
	if _, _, err := net.SplitHostPort(sockAddress); err != nil {
		return fmt.Errorf("socket address must be of the form 'host:port'")
	}
	return nil
}
