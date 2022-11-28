package config

import coreconfig "github.com/DataDog/datadog-agent/pkg/config"

var VulnProbeConfig Config

type Config struct {
	Enabled    bool
	PolicyPath string
}

func InitVulnProbeConfig() {
	VulnProbeConfig.Enabled = coreconfig.Datadog.GetBool("runtime_security_config.vulnprobe.enabled")
	VulnProbeConfig.PolicyPath = coreconfig.Datadog.GetString("runtime_security_config.vulnprobe.policy.path")
}
