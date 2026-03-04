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
	value   interface{}
	oid     string
	profile string
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

func profileDefinitions(profiles []string) []profiledefinition.ProfileDefinition {
	var p []profiledefinition.ProfileDefinition

	for _, profile := range profiles {
		profileDefs, err := snmp.GetProfileDefinition(profile)
		//improve error message
		if err != nil {
			fmt.Printf("Unable to parse profiles")
		}
		p = append(p, profileDefs)
	}
	return p
}

func normalizeOID(oid string) string {
	newOID := strings.TrimPrefix(oid, ".")
	return newOID
}
func oidMap(profiles []profiledefinition.ProfileDefinition) map[string]string {
	metricMap := make(map[string]string)
	// Go through symbol

	for _, profile := range profiles {
		metrics := profile.Metrics
		for _, metric := range metrics {
			oid := normalizeOID(metric.Symbol.OID)
			if oid != "" {
				metricMap[oid] = profile.Name
			}
		}
		// Go through symbols

		for _, metric := range metrics {
			for _, symbol := range metric.Symbols {
				oid := normalizeOID(symbol.OID)
				if oid != "" {
					metricMap[oid] = profile.Name
				}
			}
		}
		// Go through metric tags (per-metric tags, e.g. table column tags)

		for _, metric := range metrics {
			for _, metricTag := range metric.MetricTags {
				oid := normalizeOID(metricTag.OID)
				if oid != "" {
					metricMap[oid] = profile.Name
				}
			}
		}
		// Profile-level metric tags (e.g. _base has sysName at 1.3.6.1.2.1.1.5.0 here, not in metrics)

		profileTags := profile.MetricTags
		for _, tag := range profileTags {
			oid := normalizeOID(tag.Symbol.OID)
			if oid != "" {
				metricMap[oid] = profile.Name
			}
		}
	}

	return metricMap
}

// Analyze runs analysis on the first walk (pdus) using the given sysObjectID to resolve profile.
func Analyze(pdus []gosnmp.SnmpPDU, sysOID string) ([]MetricProfile, error) {
	//ToDO: correct return types to state what they are, Will also have a device profile type
	profileDef, err := FindProfile(sysOID)
	if err != nil {
		fmt.Printf("profile lookup: %v\n", err)
		return []MetricProfile{}, err
	}

	var profileList []profiledefinition.ProfileDefinition

	extendedProfiles := profileDef.Extends
	extendedProfileDefs := profileDefinitions(extendedProfiles)

	profileList = append(profileList, profileDef)
	profileList = append(profileList, extendedProfileDefs...)

	oids := pdus

	var foundMetrics []MetricProfile

	metricMap := oidMap(profileList)

	for _, oid := range oids {
		if profileName, found := metricMap[normalizeOID(oid.Name)]; found {
			foundMetrics = append(foundMetrics, MetricProfile{
				value:   oid.Value,
				oid:     oid.Name,
				profile: profileName,
			})
		}
	}

	return foundMetrics, nil
}
