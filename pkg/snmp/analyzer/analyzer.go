// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package analyzer matches SNMP walk OIDs against device profiles and formats analysis output.
package analyzer

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
)

const _cachedSysObjID = ".1.3.6.1.2.1.1.2"

type MetricProfile struct {
	Value       interface{} // SNMP value (e.g. string, uint32)
	OID         string      // full OID from the walk
	Profile     string      // profile name that defines this OID
	InterfaceID string      // e.g. "1" or "1.2" for table rows; empty for scalars/tags
	SymbolName  string      // metric/symbol name from profile (e.g. sysName, ifInOctets)
}

// SysObjectOID returns the OID to walk to fetch sysObjectID (e.g. for a fallback walk).
func SysObjectOID() string {
	return _cachedSysObjID
}

func FindSysOID(pdus []gosnmp.SnmpPDU) string {
	base := normalizeOID(_cachedSysObjID)
	with0 := base + ".0"
	for _, pdu := range pdus {
		n := normalizeOID(pdu.Name)
		if n == base || n == with0 {
			value, err := gosnmplib.GetValueFromPDU(pdu)
			if err != nil {
				value = pdu.Value
			}
			s, err := gosnmplib.StandardTypeToString(value)
			if err != nil {
				continue
			}
			return s
		}
	}
	return ""
}

// FindProfile returns the profile definition for a device given its sysObjectID.
func FindProfile(sysOID string) (profiledefinition.ProfileDefinition, error) {
	var empty profiledefinition.ProfileDefinition
	if sysOID == "" {
		return empty, errors.New("no sys object id available")
	}
	return snmp.BuildProfileForSysObjectID(sysOID)
}

func normalizeOID(oid string) string {
	s := strings.TrimPrefix(oid, ".")
	s = strings.TrimSpace(s)
	return strings.TrimSuffix(s, ".")
}

// columnBase is a table column base OID, profile, and symbol name for fast prefix lookup.
type columnBase struct {
	OID     string
	Profile string
	Name    string
}

// oidMap builds maps of normalized OID -> profile and OID -> symbol name, and a slice of column bases for prefix matching.
func oidMap(profileDefs []profiledefinition.ProfileDefinition) (profileByOID, nameByOID map[string]string, columnBases []columnBase) {
	profileByOID = make(map[string]string)
	nameByOID = make(map[string]string)
	columnOIDsAdded := make(map[string]bool)

	for _, profileDef := range profileDefs {
		profileName := profileDef.Name
		// Scalar metrics: single OID per metric (e.g. sysUpTimeInstance).
		for _, metric := range profileDef.Metrics {
			oid := normalizeOID(metric.Symbol.OID)
			if oid != "" {
				profileByOID[oid] = profileName
				if metric.Symbol.Name != "" {
					nameByOID[oid] = metric.Symbol.Name
				}
			}
			// Table column metrics: base OID for each column (e.g. ifDescr, ifInOctets).
			for _, columnSymbol := range metric.Symbols {
				oid := normalizeOID(columnSymbol.OID)
				if oid != "" {
					profileByOID[oid] = profileName
					if !columnOIDsAdded[oid] {
						columnOIDsAdded[oid] = true
						columnBases = append(columnBases, columnBase{OID: oid, Profile: profileName, Name: columnSymbol.Name})
					}
				}
			}
			// Per-metric tag OIDs (tags defined on this metric).
			for _, tagConfig := range metric.MetricTags {
				oid := normalizeOID(tagConfig.Symbol.OID)
				if oid != "" {
					profileByOID[oid] = profileName
					if tagConfig.Symbol.Name != "" {
						nameByOID[oid] = tagConfig.Symbol.Name
					}
				}
			}
		}
		// Profile-level tag OIDs (e.g. sysName, device tags).
		for _, tagConfig := range profileDef.MetricTags {
			oid := normalizeOID(tagConfig.Symbol.OID)
			if oid != "" {
				profileByOID[oid] = profileName
				if tagConfig.Symbol.Name != "" {
					nameByOID[oid] = tagConfig.Symbol.Name
				}
			}
		}
	}
	sort.Slice(columnBases, func(i, j int) bool { return len(columnBases[i].OID) > len(columnBases[j].OID) })
	return profileByOID, nameByOID, columnBases
}
func isInterfaceIndex(oid string) bool {
	return strings.HasPrefix(oid, "1.3.6.1.2.1.2.") ||
		strings.HasPrefix(oid, "1.3.6.1.2.1.31.")
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
	if strings.TrimSpace(sysOID) == "" {
		return nil, nil, "", nil, errors.New("sysObjectID is required for analysis")
	}
	// Resolve the profile definition for this device from sysObjectID.
	profileDef, err := FindProfile(normalizeOID(sysOID))
	if err != nil {
		return nil, nil, "", nil, err
	}
	profileName = profileDef.Name

	extendedProfiles = profileDef.Extends
	allProfileDefs := []profiledefinition.ProfileDefinition{profileDef}

	// Map each known OID to the profile name; columnBases are used only for prefix (table instance) lookup.
	profileByOID, nameByOID, columnBases := oidMap(allProfileDefs)

	const maxResults = 1_000_000
	var foundMetrics []MetricProfile
	var notFoundMetrics []MetricProfile
	// Match each walk PDU's OID: exact lookup first, then prefix match over column bases only (longest first).
	for _, pdu := range pdus {
		if len(foundMetrics)+len(notFoundMetrics) >= maxResults {
			fmt.Fprintln(os.Stderr, "Analysis is limited to 1,000,000 OIDs")
			break
		}
		normalizedOID := normalizeOID(pdu.Name)
		var matchedProfile string
		var symbolName string
		var interfaceID string
		if p, ok := profileByOID[normalizedOID]; ok {
			matchedProfile = p
			symbolName = nameByOID[normalizedOID]
		} else {
			// Table column instance: find longest matching column base (columnBases is sorted longest first).
			for _, base := range columnBases {
				if strings.HasPrefix(normalizedOID, base.OID+".") {
					matchedProfile = base.Profile
					symbolName = base.Name
					if isInterfaceIndex(base.OID) {
						interfaceID = strings.TrimPrefix(normalizedOID, base.OID+".")
					}
					break
				}
			}
		}
		m := MetricProfile{
			Value:       pdu.Value,
			OID:         pdu.Name,
			Profile:     matchedProfile,
			InterfaceID: interfaceID,
			SymbolName:  symbolName,
		}
		if matchedProfile != "" {
			foundMetrics = append(foundMetrics, m)
		} else {
			notFoundMetrics = append(notFoundMetrics, m)
		}
	}

	return foundMetrics, notFoundMetrics, profileName, extendedProfiles, nil
}

const reportWidth = 100

const (
	wColOID     = 28
	wColMetric  = 14
	wColIface   = 8 // "IF INDEX"
	wColValue   = 26
	wColFound   = 19
	wUnmatchedA = 49
	wUnmatchedB = 50
)

func FormatReport(found, notFound []MetricProfile, profileName string, extendedProfiles []string) string {
	var b strings.Builder
	total := len(found) + len(notFound)
	extendedStr := strings.Join(extendedProfiles, ", ")

	rule := strings.Repeat("=", reportWidth)
	dash := strings.Repeat("-", reportWidth)

	b.WriteString(rule + "\n")
	b.WriteString(center("SNMP Walk Analysis", reportWidth) + "\n")
	b.WriteString(dash + "\n")
	b.WriteString(fmt.Sprintf("  Total OIDs: %d  |  Matched: %d  |  Unmatched: %d\n", total, len(found), len(notFound)))
	b.WriteString(fmt.Sprintf("  Profile: %s\n", profileName))
	writeExtendedProfilesWrapped(&b, extendedStr, reportWidth)
	b.WriteString(rule + "\n\n")

	b.WriteString("Matched OIDs\n")
	b.WriteString(dash + "\n")
	b.WriteString(fmt.Sprintf(
		"%-*s %-*s %-*s  %-*s %-*s\n",
		wColOID, reportTruncate("OID", wColOID),
		wColMetric, reportTruncate("METRIC", wColMetric),
		wColIface, reportTruncate("IF INDEX", wColIface),
		wColValue, reportTruncate("VALUE", wColValue),
		wColFound, reportTruncate("FOUND IN", wColFound),
	))
	for _, m := range found {
		line := fmt.Sprintf(
			"%-*s %-*s %-*s  %-*s %-*s",
			wColOID, reportTruncate(m.OID, wColOID),
			wColMetric, reportTruncate(m.SymbolName, wColMetric),
			wColIface, reportTruncate(m.InterfaceID, wColIface),
			wColValue, formatReportValue(m.Value, wColValue),
			wColFound, reportTruncate(m.Profile, wColFound),
		)
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteString("\nUnmatched OIDs\n")
	b.WriteString(dash + "\n")
	b.WriteString(fmt.Sprintf(
		"%-*s %-*s\n",
		wUnmatchedA, reportTruncate("OID", wUnmatchedA),
		wUnmatchedB, reportTruncate("VALUE", wUnmatchedB),
	))
	for _, m := range notFound {
		line := fmt.Sprintf(
			"%-*s %-*s",
			wUnmatchedA, reportTruncate(m.OID, wUnmatchedA),
			wUnmatchedB, formatReportValue(m.Value, wUnmatchedB),
		)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	return b.String()
}

func writeExtendedProfilesWrapped(b *strings.Builder, joined string, width int) {
	const prefix = "  Extended Profiles: "
	if width < 40 {
		width = 40
	}
	if joined == "" {
		b.WriteString(prefix)
		b.WriteByte('\n')
		return
	}
	max := width - len(prefix)
	if max < 8 {
		max = 8
	}
	cont := strings.Repeat(" ", len(prefix))
	items := strings.Split(joined, ", ")
	var cur []string
	isFirst := true
	i := 0
	for i < len(items) {
		item := items[i]
		var test string
		if len(cur) == 0 {
			test = item
		} else {
			test = strings.Join(append(append([]string{}, cur...), item), ", ")
		}
		if len(test) <= max {
			cur = append(cur, item)
			i++
			continue
		}
		if len(cur) > 0 {
			if isFirst {
				b.WriteString(prefix)
				isFirst = false
			} else {
				b.WriteString(cont)
			}
			b.WriteString(strings.Join(cur, ", "))
			b.WriteByte('\n')
			cur = nil
			continue
		}
		// Single item does not fit on one line.
		chunk := item
		if len(chunk) > max {
			chunk = chunk[:max-3] + "..."
		}
		if isFirst {
			b.WriteString(prefix)
			isFirst = false
		} else {
			b.WriteString(cont)
		}
		b.WriteString(chunk)
		b.WriteByte('\n')
		i++
	}
	if len(cur) > 0 {
		if isFirst {
			b.WriteString(prefix)
		} else {
			b.WriteString(cont)
		}
		b.WriteString(strings.Join(cur, ", "))
		b.WriteByte('\n')
	}
}

func reportNormalizeForDisplay(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}

func formatReportValue(v interface{}, max int) string {
	s, err := gosnmplib.StandardTypeToString(v)
	if err != nil {
		s = fmt.Sprintf("%v", v)
	}
	s = reportNormalizeForDisplay(s)
	if len(s) > max {
		if max <= 3 {
			return s[:max]
		}
		return s[:max-3] + "..."
	}
	return s
}

func center(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	pad := (w - len(s)) / 2
	return strings.Repeat(" ", pad) + s + strings.Repeat(" ", w-pad-len(s))
}

func reportTruncate(s string, max int) string {
	s = reportNormalizeForDisplay(s)
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
