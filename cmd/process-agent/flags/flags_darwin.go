// +build darwin

package flags

const (
	// DefaultConfPath points to the location of datadog.yaml
	DefaultConfPath = "/opt/datadog-agent/etc/datadog.yaml"
	// DefaultSysProbeConfPath is set to empty since system-probe is not yet supported on darwin
	DefaultSysProbeConfPath = ""
)
