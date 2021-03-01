// +build !windows

package config

import (
	"fmt"
	"path/filepath"
)

const (
	defaultLogFilePath = "/var/log/datadog/process-agent.log"

	// defaultSystemProbeLogFilePath is the default logging file for the system probe
	defaultSystemProbeLogFilePath = "/var/log/datadog/system-probe.log"

	// Agent 6
	defaultDDAgentBin = "/opt/datadog-agent/bin/agent/agent"

	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"
)

// ValidateSysprobeSocket validates that the sysprobe socket config option is of the correct format.
func ValidateSysprobeSocket(sockPath string) error {
	if !filepath.IsAbs(sockPath) {
		return fmt.Errorf("it must be an absolute file path")
	}
	return nil
}
