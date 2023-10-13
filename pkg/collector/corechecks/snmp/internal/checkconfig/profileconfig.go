package checkconfig

import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"

var globalProfileConfigMap profile.ProfileConfigMap

func SetGlobalProfileConfigMap(configMap profile.ProfileConfigMap) {
	globalProfileConfigMap = configMap
}
