package haagent

import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

func IsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("ha_agent.enabled")
}

func GetGroup() string {
	return pkgconfigsetup.Datadog().GetString("ha_agent.group")
}
