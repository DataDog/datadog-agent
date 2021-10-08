//+build linux

package config

import (
	"fmt"
	"path/filepath"
)

const (
	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"

	defaultConfigDir = "/etc/datadog-agent"
)

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockPath string) error {
	if !filepath.IsAbs(sockPath) {
		return fmt.Errorf("socket path must be an absolute file path: %s", sockPath)
	}
	return nil
}
