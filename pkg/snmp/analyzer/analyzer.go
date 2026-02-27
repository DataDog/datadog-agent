package analyzer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/gosnmp/gosnmp"
)

const _cached_sys_obj_id = ".1.3.6.1.2.1.1.2"

// SysObjectOID returns the OID to walk to fetch sysObjectID (e.g. for a fallback walk).
func SysObjectOID() string {
	return _cached_sys_obj_id
}

func FindSysOID(pdus []gosnmp.SnmpPDU) string {
	for _, pdu := range pdus {
		if pdu.Name == _cached_sys_obj_id {
			return fmt.Sprintf("%v", pdu.Value)
		}
	}
	return ""
}

// ProfileFromSysOID returns the profile definition for a device given its sysObjectID.
func ProfileFromSysOID(sysOID string) (profiledefinition.ProfileDefinition, error) {
	var empty profiledefinition.ProfileDefinition
	if sysOID == "" {
		return empty, fmt.Errorf("no sys object id available")
	}

	provider, _, err := profile.GetProfileProvider(profile.ProfileConfigMap{})
	if err != nil {
		return empty, fmt.Errorf("failed to load profiles: %w", err)
	}

	cfg := &checkconfig.CheckConfig{
		ProfileProvider:     provider,
		ProfileName:         checkconfig.ProfileNameAuto,
		RequestedMetrics:    nil,
		RequestedMetricTags: nil,
		CollectTopology:     false,
		CollectVPN:          false,
	}

	profileDef, err := cfg.BuildProfile(sysOID)
	if err != nil {
		return profileDef, err
	}

	return profileDef, nil
}

// findExtendedProfiles returns the list of extended profile names for the given profile definition.
func findExtendedProfiles(profileDef profiledefinition.ProfileDefinition) ([]string, error) {
	provider, _, err := profile.GetProfileProvider(profile.ProfileConfigMap{})
	if err != nil {
		return nil, fmt.Errorf("unable to find extended profiles due to: %w", err)
	}

	profileConfig := provider.GetProfile(profileDef.Name)
	if profileConfig == nil {
		return nil, nil
	}

	return profileConfig.Definition.Extends, nil
}
