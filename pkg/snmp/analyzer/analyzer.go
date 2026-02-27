package analyzer

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
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

// FindProfile returns the profile definition for a device given its sysObjectID.
// It uses the public SNMP check API (no internal packages).
func FindProfile(sysOID string) (profiledefinition.ProfileDefinition, error) {
	var empty profiledefinition.ProfileDefinition
	if sysOID == "" {
		return empty, fmt.Errorf("no sys object id available")
	}
	return snmp.BuildProfileForSysObjectID(sysOID)
}

// FindExtendedProfiles returns the list of extended profile names for the given profile definition.
// It uses the public SNMP check API (no internal packages).
func FindExtendedProfiles(profileDef profiledefinition.ProfileDefinition) ([]string, error) {
	return snmp.GetExtendedProfileNames(profileDef.Name)
}

// Analyze runs analysis on the first walk (pdus) using the given sysObjectID to resolve profile.
func Analyze(pdus []gosnmp.SnmpPDU, sysOID string) {
	profileDef, err := FindProfile(sysOID)
	if err != nil {
		fmt.Printf("profile lookup: %v\n", err)
		return
	}
	_ = profileDef // use profileDef.Name, profileDef.Metrics, etc.
	// TODO: analyze pdus in context of profile (e.g. which OIDs match profile, suggestions)
	_ = pdus
}
