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
	SymbolName  string      // metric/symbol name from profile (e.g. sysName, ifInOctets)
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
	profileByOID, nameByOID, columnBases := oidMap(allProfileDefs)

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

func FormatReport(found, notFound []MetricProfile, profileName string, extendedProfiles []string) string {
	var b strings.Builder
	total := len(found) + len(notFound)
	extendedStr := strings.Join(extendedProfiles, ", ")

	// Header box
	b.WriteString("╔" + strings.Repeat("═", reportWidth-2) + "╗\n")
	inner := reportWidth - 2
	b.WriteString("║" + center("SNMP Walk Analysis", inner) + "║\n")
	b.WriteString("╠" + strings.Repeat("═", inner) + "╣\n")
	b.WriteString("║ " + padRight(fmt.Sprintf("Total OIDs: %d   │ Matched: %d    │ Unmatched: %d", total, len(found), len(notFound)), inner-2) + " ║\n")
	b.WriteString("║ " + padRight("Profile: "+profileName, inner-2) + " ║\n")
	b.WriteString("║ " + padRight("Extended Profiles: "+extendedStr, inner-2) + " ║\n")
	b.WriteString("╚" + strings.Repeat("═", reportWidth-2) + "╝\n\n")

	// Matched table
	b.WriteString("┌─ Matched OIDs " + strings.Repeat("─", reportWidth-18) + "┐\n")
	b.WriteString("│ OID" + strings.Repeat(" ", 35) + "│ Metric Name" + strings.Repeat(" ", 6) + "│ Interface  │ Value" + strings.Repeat(" ", 20) + "│ Found In" + strings.Repeat(" ", 10) + "│\n")
	b.WriteString("├" + strings.Repeat("─", 37) + "┼" + strings.Repeat("─", 18) + "┼" + strings.Repeat("─", 12) + "┼" + strings.Repeat("─", 25) + "┼" + strings.Repeat("─", 19) + "┤\n")
	for _, m := range found {
		valStr := fmt.Sprintf("%v", m.Value)
		if len(valStr) > 22 {
			valStr = valStr[:19] + "..."
		}
		iface := m.InterfaceID
		if iface == "" {
			iface = "0"
		}
		b.WriteString("│ " + padRight(truncate(m.OID, 35), 35) + " │ " + padRight(truncate(m.SymbolName, 16), 16) + " │ " + padRight(iface, 10) + " │ " + padRight(valStr, 23) + " │ " + padRight(truncate(m.Profile, 17), 17) + " │\n")
	}
	b.WriteString("└" + strings.Repeat("─", 37) + "┴" + strings.Repeat("─", 18) + "┴" + strings.Repeat("─", 12) + "┴" + strings.Repeat("─", 25) + "┴" + strings.Repeat("─", 19) + "┘\n\n")

	// Unmatched table
	b.WriteString("┌─ Unmatched OIDs " + strings.Repeat("─", reportWidth-20) + "┐\n")
	b.WriteString("│ OID" + strings.Repeat(" ", 48) + "│ Interface  │ Value" + strings.Repeat(" ", 38) + "│\n")
	b.WriteString("├" + strings.Repeat("─", 50) + "┼" + strings.Repeat("─", 12) + "┼" + strings.Repeat("─", 43) + "┤\n")
	for _, m := range notFound {
		valStr := fmt.Sprintf("%v", m.Value)
		if len(valStr) > 40 {
			valStr = valStr[:37] + "..."
		}
		iface := m.InterfaceID
		if iface == "" {
			iface = "0"
		}
		b.WriteString("│ " + padRight(truncate(m.OID, 48), 48) + " │ " + padRight(iface, 10) + " │ " + padRight(valStr, 41) + " │\n")
	}
	b.WriteString("└" + strings.Repeat("─", 50) + "┴" + strings.Repeat("─", 12) + "┴" + strings.Repeat("─", 43) + "┘\n")
	return b.String()
}

func center(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	pad := (w - len(s)) / 2
	return strings.Repeat(" ", pad) + s + strings.Repeat(" ", w-pad-len(s))
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return truncate(s, w)
	}
	return s + strings.Repeat(" ", w-len(s))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
