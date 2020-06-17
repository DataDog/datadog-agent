// +build !windows

package config

const (
	defaultLogFilePath = "/var/log/datadog/process-agent.log"

	// Agent 6
	defaultDDAgentBin = "/opt/datadog-agent/bin/agent/agent"

	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"
)
