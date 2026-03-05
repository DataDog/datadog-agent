package analyzer

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/gosnmp/gosnmp"
)

const _cached_sys_obj_id = ".1.3.6.1.2.1.1.2"

type MetricProfile struct {
	Value       interface{} // SNMP value (e.g. string, uint32)
	OID         string      // full OID from the walk
	Profile     string      // profile name that defines this OID
	InterfaceID string      // e.g. "1" or "1.2" for table rows; empty for scalars/tags
}

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
func FindProfile(sysOID string) (profiledefinition.ProfileDefinition, error) {
	var empty profiledefinition.ProfileDefinition
	if sysOID == "" {
		return empty, fmt.Errorf("no sys object id available")
	}
	return snmp.BuildProfileForSysObjectID(sysOID)
}

// profileDefinitions returns profile definitions for the given profile names (e.g. extended profile names).
func profileDefinitions(profileNames []string) []profiledefinition.ProfileDefinition {
	var defs []profiledefinition.ProfileDefinition
	for _, name := range profileNames {
		profileDef, err := snmp.GetProfileDefinition(name)
		if err != nil {
			continue
		}
		defs = append(defs, profileDef)
	}
	return defs
}

func normalizeOID(oid string) string {
	newOID := strings.TrimPrefix(oid, ".")
	return newOID
}

// oidMap builds a map of normalized OID -> profile name from the given profile definitions.
func oidMap(profileDefs []profiledefinition.ProfileDefinition) map[string]string {
	oidToProfile := make(map[string]string)

	for _, profileDef := range profileDefs {
		profileName := profileDef.Name
		// Scalar metrics: single OID per metric (e.g. sysUpTimeInstance).
		for _, metric := range profileDef.Metrics {
			oid := normalizeOID(metric.Symbol.OID)
			if oid != "" {
				oidToProfile[oid] = profileName
			}
			// Table column metrics: base OID for each column (e.g. ifDescr, ifInOctets).
			for _, columnSymbol := range metric.Symbols {
				oid := normalizeOID(columnSymbol.OID)
				if oid != "" {
					oidToProfile[oid] = profileName
				}
			}
			// Per-metric tag OIDs (tags defined on this metric).
			for _, tagConfig := range metric.MetricTags {
				oid := normalizeOID(tagConfig.Symbol.OID)
				if oid != "" {
					oidToProfile[oid] = profileName
				}
			}
		}
		// Profile-level tag OIDs (e.g. sysName, device tags).
		for _, tagConfig := range profileDef.MetricTags {
			oid := normalizeOID(tagConfig.Symbol.OID)
			if oid != "" {
				oidToProfile[oid] = profileName
			}
		}
	}
	return oidToProfile
}

// Analyze runs analysis on the walk results (pdus) using the given sysObjectID to resolve the device profile.
// It returns matched metrics, unmatched metrics (OIDs not in any profile), the resolved profile name, extended profile names, and an error if profile lookup fails.
func Analyze(pdus []gosnmp.SnmpPDU, sysOID string) (
	found []MetricProfile,
	notFound []MetricProfile,
	profileName string,
	extendedProfiles []string,
	err error,
) {
	// Resolve the profile definition for this device from sysObjectID.
	profileDef, err := FindProfile(sysOID)
	if err != nil {
		return nil, nil, "", nil, err
	}
	profileName = profileDef.Name

	// Build list of profile definitions: root profile first, then extended profiles (e.g. _base, _generic-if).
	extendedProfiles = profileDef.Extends
	extendedProfileDefs := profileDefinitions(profileDef.Extends)
	var allProfileDefs []profiledefinition.ProfileDefinition
	allProfileDefs = append(allProfileDefs, profileDef)
	allProfileDefs = append(allProfileDefs, extendedProfileDefs...)

	// Map each known OID to the profile name that defines it (so we can match walk PDUs to the right profile).
	oidToProfile := oidMap(allProfileDefs)

	var foundMetrics []MetricProfile
	var notFoundMetrics []MetricProfile
	// Match each walk PDU's OID against the profile OID map (exact match, or prefix match for table columns).
	for _, pdu := range pdus {
		normalizedOID := normalizeOID(pdu.Name)
		var matchedProfile string
		var interfaceID string
		if p, ok := oidToProfile[normalizedOID]; ok {
			matchedProfile = p
		} else {
			// Table column instance: PDU OID is column base + index (e.g. 1.3.6.1.2.1.2.2.1.2.1). Find which column base it belongs to; keep the longest match so we get the most specific column.
			longestMatchLen := 0
			var longestMatchingBaseOID string
			for baseOID, p := range oidToProfile {
				if strings.HasPrefix(normalizedOID, baseOID+".") && len(baseOID) > longestMatchLen {
					longestMatchLen = len(baseOID)
					longestMatchingBaseOID = baseOID
					matchedProfile = p
				}
			}
			if longestMatchingBaseOID != "" {
				interfaceID = normalizedOID[len(longestMatchingBaseOID)+1:] // suffix after "baseOID."
			} else if idx := strings.LastIndex(normalizedOID, "."); idx >= 0 {
				// Unmatched OID: use last component as interface/index (e.g. "1" from "1.3.6.1.2.1.2.2.1.2.1").
				interfaceID = normalizedOID[idx+1:]
			}
		}
		m := MetricProfile{
			Value:       pdu.Value,
			OID:         pdu.Name,
			Profile:     matchedProfile,
			InterfaceID: interfaceID,
		}
		if matchedProfile != "" {
			foundMetrics = append(foundMetrics, m)
		} else {
			notFoundMetrics = append(notFoundMetrics, m)
		}
	}

	return foundMetrics, notFoundMetrics, profileName, extendedProfiles, nil
}
