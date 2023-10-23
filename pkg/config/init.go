package config

import (
	"strings"
)

// Datadog is the global configuration object
var (
	Datadog     Config
	SystemProbe Config
)

func init() {
	osinit()
	// Configure Datadog global configuration
	Datadog = NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	SystemProbe = NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	// Configuration defaults
	InitConfig(Datadog)
	InitSystemProbeConfig(SystemProbe)
}
