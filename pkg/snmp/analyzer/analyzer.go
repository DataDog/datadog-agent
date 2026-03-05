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

// profileDefinitions returns profile definitions for the given profile names (e.g. extended profile names).
func profileDefinitions(profileNames []string) []profiledefinition.ProfileDefinition {
	var defs []profiledefinition.ProfileDefinition
	for _, name := range profileNames {
		profileDef, err := snmp.GetProfileDefinition(name)
		if err != nil {
			fmt.Printf("Unable to parse profile %v", name)
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
