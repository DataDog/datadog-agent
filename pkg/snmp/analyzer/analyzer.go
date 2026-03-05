package analyzer

import (
	"fmt"
	"sort"
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
	sysOIDNorm := normalizeOID(_cached_sys_obj_id)
	for _, pdu := range pdus {
		if normalizeOID(pdu.Name) == sysOIDNorm {
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

// profileDefinitions returns profile definitions for the given profile names. Returns error if any profile fails to load.
func profileDefinitions(profileNames []string) ([]profiledefinition.ProfileDefinition, error) {
	var defs []profiledefinition.ProfileDefinition
	for _, name := range profileNames {
		profileDef, err := snmp.GetProfileDefinition(name)
		if err != nil {
			return nil, fmt.Errorf("profile %s: %w", name, err)
		}
		defs = append(defs, profileDef)
	}
	return defs, nil
}

func normalizeOID(oid string) string {
	s := strings.TrimPrefix(oid, ".")
	s = strings.TrimSpace(s)
	return strings.TrimSuffix(s, ".")
}

// columnBase is a table column base OID and its profile, used for fast prefix lookup.
type columnBase struct {
	OID     string
	Profile string
}

// oidMap builds a map of normalized OID -> profile name and a slice of column-base OIDs for prefix matching.
// Column bases are sorted by OID length descending so the longest prefix match can be found first.
func oidMap(profileDefs []profiledefinition.ProfileDefinition) (profileByOID map[string]string, columnBases []columnBase) {
	profileByOID = make(map[string]string)
	columnOIDsAdded := make(map[string]bool)

	for _, profileDef := range profileDefs {
		profileName := profileDef.Name
		// Scalar metrics: single OID per metric (e.g. sysUpTimeInstance).
		for _, metric := range profileDef.Metrics {
			oid := normalizeOID(metric.Symbol.OID)
			if oid != "" {
				profileByOID[oid] = profileName
			}
			// Table column metrics: base OID for each column (e.g. ifDescr, ifInOctets).
			for _, columnSymbol := range metric.Symbols {
				oid := normalizeOID(columnSymbol.OID)
				if oid != "" {
					profileByOID[oid] = profileName
					if !columnOIDsAdded[oid] {
						columnOIDsAdded[oid] = true
						columnBases = append(columnBases, columnBase{OID: oid, Profile: profileName})
					}
				}
			}
			// Per-metric tag OIDs (tags defined on this metric).
			for _, tagConfig := range metric.MetricTags {
				oid := normalizeOID(tagConfig.Symbol.OID)
				if oid != "" {
					profileByOID[oid] = profileName
				}
			}
		}
		// Profile-level tag OIDs (e.g. sysName, device tags).
		for _, tagConfig := range profileDef.MetricTags {
			oid := normalizeOID(tagConfig.Symbol.OID)
			if oid != "" {
				profileByOID[oid] = profileName
			}
		}
	}
	// Longest first so first prefix match is the most specific column.
	sort.Slice(columnBases, func(i, j int) bool { return len(columnBases[i].OID) > len(columnBases[j].OID) })
	return profileByOID, columnBases
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
	extendedProfileDefs, err := profileDefinitions(profileDef.Extends)
	if err != nil {
		return nil, nil, "", nil, err
	}
	var allProfileDefs []profiledefinition.ProfileDefinition
	allProfileDefs = append(allProfileDefs, profileDef)
	allProfileDefs = append(allProfileDefs, extendedProfileDefs...)

	// Map each known OID to the profile name; columnBases are used only for prefix (table instance) lookup.
	profileByOID, columnBases := oidMap(allProfileDefs)

	const maxResults = 100_000
	var foundMetrics []MetricProfile
	var notFoundMetrics []MetricProfile
	// Match each walk PDU's OID: exact lookup first, then prefix match over column bases only (longest first).
	for _, pdu := range pdus {
		if len(foundMetrics)+len(notFoundMetrics) >= maxResults {
			continue
		}
		normalizedOID := normalizeOID(pdu.Name)
		var matchedProfile string
		var interfaceID string
		if p, ok := profileByOID[normalizedOID]; ok {
			matchedProfile = p
		} else {
			// Table column instance: find longest matching column base (columnBases is sorted longest first).
			for _, base := range columnBases {
				if strings.HasPrefix(normalizedOID, base.OID+".") {
					matchedProfile = base.Profile
					interfaceID = normalizedOID[len(base.OID)+1:] // suffix after "base."
					break
				}
			}
			if matchedProfile == "" {
				if dotIdx := strings.LastIndex(normalizedOID, "."); dotIdx >= 0 {
					interfaceID = normalizedOID[dotIdx+1:]
				}
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
