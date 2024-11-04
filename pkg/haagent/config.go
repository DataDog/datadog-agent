package haagent

import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

func getHAIntegrations() []string {
	return pkgconfigsetup.Datadog().GetStringSlice("ha_agent.integrations")
}

func IsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("ha_agent.enabled")
}

func GetGroup() string {
	return pkgconfigsetup.Datadog().GetString("ha_agent.group")
}

func IsHAIntegration(integrationName string) bool {
	for _, name := range getHAIntegrations() {
		if name == integrationName {
			return true
		}
	}
	return false
}
