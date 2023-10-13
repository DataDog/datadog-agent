package checkconfig

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

func getProfiles(initConfigProfiles profile.ProfileConfigMap) (profile.ProfileConfigMap, error) {
	var profiles profile.ProfileConfigMap
	if len(initConfigProfiles) > 0 {
		// TODO: [PERFORMANCE] Load init config custom profiles once for all integrations
		//   There are possibly multiple init configs
		customProfiles, err := loadProfiles(initConfigProfiles)
		if err != nil {
			return nil, fmt.Errorf("failed to load initConfig profiles: %s", err)
		}
		profiles = customProfiles
	} else if profileBundleFileExist() {
		defaultProfiles, err := loadBundleJSONProfiles()
		if err != nil {
			return nil, fmt.Errorf("failed to load bundle json profiles: %s", err)
		}
		profiles = defaultProfiles
	} else {
		defaultProfiles, err := loadYamlProfiles()
		if err != nil {
			return nil, fmt.Errorf("failed to load yaml profiles: %s", err)
		}
		profiles = defaultProfiles
	}
	for _, profileDef := range profiles {
		profiledefinition.NormalizeMetrics(profileDef.Definition.Metrics)
	}
	return profiles, nil
}
