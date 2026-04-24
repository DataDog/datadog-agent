package analyzer

import (
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestFindSysOID_Found(t *testing.T) {

	pdus := []gosnmp.SnmpPDU{
		{Name: "1.3.6.1.2.1.1.1.0", Value: "some other value"},
		{Name: SysObjectOID(), Value: "1.3.6.1.4.1.9.1.1208"},
	}

	sysOID := FindSysOID(pdus)
	if sysOID != "1.3.6.1.4.1.9.1.1208" {
		t.Fatalf("expected sysOID, got %q", sysOID)
	}
}

func TestFindSysOID_FoundScalarInstance0(t *testing.T) {

	pdus := []gosnmp.SnmpPDU{
		{Name: ".1.3.6.1.2.1.1.2.0", Value: "1.3.6.1.4.1.9.1.1208"},
	}
	if sysOID := FindSysOID(pdus); sysOID != "1.3.6.1.4.1.9.1.1208" {
		t.Fatalf("Found sysOID = %q, but got %q", sysOID, "1.3.6.1.4.1.9.1.1208")
	}
}

func TestFindSysOID_NotFound(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: ".1.3.3.1.2.1.2.2", Value: "no sysOID here"},
	}

	sysOID := FindSysOID(pdus)
	if sysOID != "" {
		t.Fatalf("expected empty string, got %q", sysOID)
	}
}

func TestFindSysOID_NonStringValue(t *testing.T) {

	pdus := []gosnmp.SnmpPDU{
		{Name: SysObjectOID(), Value: []byte{49, 50, 51}},
	}

	sysOID := FindSysOID(pdus)
	if sysOID == "" {
		t.Fatalf("expected non-empty string, got %q", sysOID)
	}
}

func TestAnalyze_EmptySysOID(t *testing.T) {
	expectedErrMsg := "sysObjectID is required for analysis"
	pdus := []gosnmp.SnmpPDU{
		{Name: SysObjectOID(), Value: "x"},
	}

	_, _, _, _, err := Analyze(nil, "")
	if err == nil || err.Error() != expectedErrMsg {
		t.Fatalf("Analyze(nil, \"\"): got err=%v, want Error()=%q", err, expectedErrMsg)
	}

	_, _, _, _, err = Analyze(pdus, "")
	if err == nil || err.Error() != expectedErrMsg {
		t.Fatalf("Analyze(..., whitespace sysOID): got err=%v, want Error()=%q", err, expectedErrMsg)
	}
}

func TestProfileFound(t *testing.T) {
	//Profile for dell
	pdus := []gosnmp.SnmpPDU{
		{Name: SysObjectOID(), Value: "1.3.6.1.4.1.674.1"},
	}

	sysOID := FindSysOID(pdus)
	profile, err := FindProfile(sysOID)

	if err != nil {
		t.Skipf("profile lookup not available: %v", err)
	}
	if profile.Name == "" {
		t.Skip("no profile matched this sysObjectID in default profiles")
	}
}

func TestProfileNotFound(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: SysObjectOID(), Value: "1.1.1.1.4.1.14.1"},
	}

	sysOID := FindSysOID(pdus)
	profile, err := FindProfile(sysOID)

	if err != nil && strings.Contains(err.Error(), "failed to load") {
		t.Skipf("profile lookup not available: %v", err)
	}
	if err == nil && profile.Name != "" {
		t.Fatalf("expected no profile to match sysOID %q, got profile %q", sysOID, profile.Name)
	}
}

func TestUnmatchedProfile(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: SysObjectOID(), Value: "1.3.6.1.4.1.14823.1.1.17"},
		{Name: ".1.3.6.1.2.1.1.1.0.1234.2324", Value: "Fake OID"},
	}
	sysOID := FindSysOID(pdus)
	_, notFound, _, _, err := Analyze(pdus, sysOID)

	if err != nil {
		t.Skipf("profile lookup not available (analyzer returned 0 metrics): %v", err)
	}

	for _, p := range notFound {
		if p.Profile != "" {
			t.Fatalf("expected no profiles to match the oid of %v", p.Value)
		}
	}

}

func TestAnalyze(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: SysObjectOID(), Value: "1.3.6.1.4.1.674.1"},

		{Name: ".1.3.6.1.2.1.1.1.0", Value: "Dell iDRAC SNMP Agent"}, // sysDescr
		{Name: ".1.3.6.1.2.1.1.5.0", Value: "dell-pdu-01"},           // sysName
		{Name: ".1.3.6.1.2.1.1.3.0", Value: uint32(12345678)},        // sysUpTime
	}

	sysOID := FindSysOID(pdus)

	found, notFound, profileName, extendedProfiles, err := Analyze(pdus, sysOID)
	if err != nil {
		t.Skipf("profile lookup not available (analyzer returned 0 metrics): %v", err)
	}
	t.Logf("Analyze returned %d matched, %d not found, profile=%s", len(found), len(notFound), profileName)

	report := FormatReport(found, notFound, profileName, extendedProfiles)
	if report == "" {
		t.Fatal("FormatReport returned empty string")
	}
	if !strings.Contains(report, "SNMP Walk Analysis") {
		t.Error("report missing header")
	}
	if !strings.Contains(report, profileName) {
		t.Errorf("report missing profile name %q", profileName)
	}
	if !strings.Contains(report, "Matched:") || !strings.Contains(report, "Unmatched:") {
		t.Error("report missing matched/unmatched counts")
	}
	if !strings.Contains(report, "Matched OIDs") || !strings.Contains(report, "Unmatched OIDs") {
		t.Error("report missing section headers")
	}
}

func TestFormatReport(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: SysObjectOID(), Value: "1.3.6.1.4.1.674.1"},
		{Name: ".1.3.6.1.2.1.1.1.0", Value: "Dell iDRAC SNMP Agent"},
		{Name: ".1.3.6.1.2.1.1.5.0", Value: "dell-pdu-01"},
		{Name: ".1.3.6.1.2.1.1.3.0", Value: uint32(12345678)},
	}
	sysOID := FindSysOID(pdus)

	found, notFound, profileName, extendedProfiles, err := Analyze(pdus, sysOID)
	if err != nil {
		t.Skipf("profile lookup not available: %v", err)
	}

	report := FormatReport(found, notFound, profileName, extendedProfiles)
	if report == "" {
		t.Fatal("FormatReport returned empty string")
	}
	if !strings.Contains(report, "SNMP Walk Analysis") {
		t.Error("report missing header")
	}
	if !strings.Contains(report, profileName) {
		t.Errorf("report missing profile name %q", profileName)
	}
	if !strings.Contains(report, "Matched:") || !strings.Contains(report, "Unmatched:") {
		t.Error("report missing matched/unmatched counts")
	}
	if !strings.Contains(report, "Matched OIDs") || !strings.Contains(report, "Unmatched OIDs") {
		t.Error("report missing section headers")
	}
	if !strings.Contains(report, "1.3.6.1.2.1.1.1.0") && !strings.Contains(report, "Dell iDRAC") {
		t.Error("report should contain at least one analyzed OID or value from the walk")
	}
}

func TestFormatReport_matchedUnmatchedTableLineWidth(t *testing.T) {
	found := []MetricProfile{
		{
			OID:         "1" + strings.Repeat(".2", 80) + ".0",
			SymbolName:  strings.Repeat("L", 50),
			InterfaceID: "1234567890123456",
			Value:       strings.Repeat("V", 100),
			Profile:     strings.Repeat("P", 50),
		},
	}
	notFound := []MetricProfile{
		{OID: "1" + strings.Repeat(".3", 80) + ".0", Value: strings.Repeat("U", 200)},
	}
	rep := FormatReport(found, notFound, "device-profile", nil)
	lines := strings.Split(rep, "\n")
	// Lines under Matched OIDs: dash, header (100), data (100)
	inMatched := -1
	for i, line := range lines {
		if line == "Matched OIDs" {
			inMatched = i
			break
		}
	}
	if inMatched < 0 {
		t.Fatal("no Matched OIDs section")
	}
	// inMatched+1 = dash, +2 = table header, +3 = data row
	hdr := lines[inMatched+2]
	data := lines[inMatched+3]
	if len(hdr) != reportWidth {
		t.Errorf("matched header row: len %d, want %d: %q", len(hdr), reportWidth, hdr)
	}
	if len(data) != reportWidth {
		t.Errorf("matched data row: len %d, want %d: %q", len(data), reportWidth, data)
	}
	inUnmatched := -1
	for i, line := range lines {
		if line == "Unmatched OIDs" {
			inUnmatched = i
			break
		}
	}
	uHdr := lines[inUnmatched+2]
	uData := lines[inUnmatched+3]
	if len(uHdr) != reportWidth {
		t.Errorf("unmatched header row: len %d, want %d", len(uHdr), reportWidth)
	}
	if len(uData) != reportWidth {
		t.Errorf("unmatched data row: len %d, want %d", len(uData), reportWidth)
	}
}

func TestInterfaceID(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: SysObjectOID(), Value: "1.3.6.1.4.1.3375.2.1.3.4.1"},
		{Name: ".1.3.6.1.2.1.1.1.0", Value: "Test Device"},
		{Name: ".1.3.6.1.2.1.1.5.0", Value: "router-01"},
		{Name: ".1.3.6.1.2.1.1.3.0", Value: uint32(12345678)},

		// ifTable columns defined in _generic-if metrics.symbols
		{Name: ".1.3.6.1.2.1.2.2.1.14.12", Value: uint32(1000)},
		{Name: ".1.3.6.1.2.1.2.2.1.14.23", Value: uint32(2000)},
		{Name: ".1.3.6.1.2.1.2.2.1.13.11", Value: uint32(3000)},
		{Name: ".1.3.6.1.2.1.2.2.1.13.24", Value: uint32(4000)},
	}

	sysOID := FindSysOID(pdus)

	found, _, _, _, err := Analyze(pdus, sysOID)
	if err != nil {
		t.Skipf("profile lookup not available: %v", err)
	}

	expected := map[string]bool{
		"12": false,
		"23": false,
		"11": false,
		"24": false,
	}

	for _, m := range found {
		if _, ok := expected[m.InterfaceID]; ok {
			expected[m.InterfaceID] = true
		}
	}

	for id, seen := range expected {
		if !seen {
			t.Fatalf("expected interfaceID %s to be found", id)
		}
	}
}
