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
	value interface{}
	oid   string
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

// FindExtendedProfiles returns the list of extended profile names for the given profile definition.
func FindExtendedProfiles(profileDef profiledefinition.ProfileDefinition) ([]string, error) {
	return snmp.GetExtendedProfileNames(profileDef.Name)
}

func normalizeOID(oid string) string {
	newOID := strings.TrimPrefix(oid, ".")
	return newOID
}
func oidMap(metrics []profiledefinition.MetricsConfig, mapType string) map[string]string {
	metricMap := make(map[string]string)

	if mapType == "symbol" {
		for _, metric := range metrics {
			oid := normalizeOID(metric.Symbol.OID)
			if oid != "" {
				metricMap[oid] = metric.Symbol.Name
			}
		}
	}

	if mapType == "symbols" {
		for _, metric := range metrics {
			for _, symbol := range metric.Symbols {
				oid := normalizeOID(symbol.OID)
				if oid != "" {
					metricMap[oid] = symbol.Name
				}

			}
		}
	}

	if mapType == "metric_tags" {
		for _, metric := range metrics {
			for _, metricTag := range metric.MetricTags {
				oid := normalizeOID(metricTag.Symbol.OID)
				if oid != "" {
					metricMap[oid] = metricTag.Symbol.Name
				}
			}
		}
	}
	return metricMap
}

func tagOidMap(metrics []profiledefinition.MetricTagConfig) map[string]string {
	metricMap := make(map[string]string)

	for _, tag := range metrics {
		oid := normalizeOID(tag.OID)
		if oid != "" {
			metricMap[oid] = tag.Symbol.Name
		}
	}

	return metricMap
}

// ToDO: Derive the profile that it extended from to match the metric.

// Analyze runs analysis on the first walk (pdus) using the given sysObjectID to resolve profile.
func Analyze(pdus []gosnmp.SnmpPDU, sysOID string) ([]MetricProfile, string, error) {
	profileDef, err := FindProfile(sysOID)
	if err != nil {
		fmt.Printf("profile lookup: %v\n", err)
		return []MetricProfile{}, "", err
	}

	//extendedProfiles, err := FindExtendedProfiles(profileDef)
	// if err != nil {
	// 	fmt.Printf("extend profile lookup: %v\n", err)
	// }
	oids := pdus
	profileMetrics := profileDef.Metrics
	profileName := profileDef.Name
	profileTags := profileDef.MetricTags
	var foundMetrics []MetricProfile

	//Go through symbol
	symbolMap := oidMap(profileMetrics, "symbol")
	for _, oid := range oids {
		if _, found := symbolMap[normalizeOID(oid.Name)]; found {
			foundMetrics = append(foundMetrics, MetricProfile{
				value: oid.Value,
				oid:   oid.Name,
			})
		}
	}

	// Go through symbols
	symbolsMap := oidMap(profileMetrics, "symbols")
	for _, oid := range oids {
		if _, found := symbolsMap[normalizeOID(oid.Name)]; found {
			foundMetrics = append(foundMetrics, MetricProfile{
				value: oid.Value,
				oid:   oid.Name,
			})
		}
	}

	// Go through metric tags (per-metric tags, e.g. table column tags)
	metricTagMap := oidMap(profileMetrics, "metric_tags")
	for _, oid := range oids {
		if _, found := metricTagMap[normalizeOID(oid.Name)]; found {
			foundMetrics = append(foundMetrics, MetricProfile{
				value: oid.Value,
				oid:   oid.Name,
			})
		}
	}

	// Profile-level metric tags (e.g. _base has sysName at 1.3.6.1.2.1.1.5.0 here, not in metrics)
	profileTagMap := tagOidMap(profileTags)
	for _, oid := range oids {
		if _, found := profileTagMap[normalizeOID(oid.Name)]; found {
			foundMetrics = append(foundMetrics, MetricProfile{
				value: oid.Value,
				oid:   oid.Name,
			})
		}
	}

	return foundMetrics, profileName, nil
}
