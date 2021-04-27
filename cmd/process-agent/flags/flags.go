// +build !windows,!darwin

package flags

const (
	// DefaultConfPath points to the location of datadog.yaml
	DefaultConfPath = "/etc/datadog-agent/datadog.yaml"
	// DefaultSysProbeConfPath points to the location of system-probe.yaml
	DefaultSysProbeConfPath = "/etc/datadog-agent/system-probe.yaml"
)
