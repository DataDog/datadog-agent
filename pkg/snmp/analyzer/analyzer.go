package analyzer

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

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
	base := normalizeOID(_cached_sys_obj_id)
	with0 := base + ".0"
	for _, pdu := range pdus {
		n := normalizeOID(pdu.Name)
		if n == base || n == with0 {
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

	const maxResults = 1_000_000
	var foundMetrics []MetricProfile
	var notFoundMetrics []MetricProfile
	// Match each walk PDU's OID: exact lookup first, then prefix match over column bases only (longest first).
	for _, pdu := range pdus {
		if len(foundMetrics)+len(notFoundMetrics) >= maxResults {
			fmt.Fprintln(os.Stderr, "Analysis is limited to 1,000,000 OIDs")
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

// FormatReport renders the analysis as plain ASCII (no Unicode box-drawing) and tabwriter-aligned
// columns so layout stays correct in terminals, IDEs, email, and web views with monospace fonts.
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
	b.WriteString(fmt.Sprintf("  Extended Profiles: %s\n", extendedStr))
	b.WriteString(rule + "\n\n")

	b.WriteString("Matched OIDs\n")
	b.WriteString(dash + "\n")
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "OID\tMETRIC\tINTERFACE INDEX\tVALUE\tFOUND IN")
	for _, m := range found {
		valStr := formatReportValue(m.Value, 32)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			truncate(m.OID, 45),
			truncate(m.SymbolName, 20),
			m.InterfaceID,
			valStr,
			truncate(m.Profile, 20),
		)
	}
	_ = tw.Flush()

	b.WriteString("\nUnmatched OIDs\n")
	b.WriteString(dash + "\n")
	tw2 := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw2, "OID\tVALUE")
	for _, m := range notFound {
		fmt.Fprintf(tw2, "%s\t%s\n", truncate(m.OID, 60), formatReportValue(m.Value, 60))
	}
	_ = tw2.Flush()
	b.WriteString("\n")
	return b.String()
}

func formatReportValue(v interface{}, max int) string {
	s := fmt.Sprintf("%v", v)
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
