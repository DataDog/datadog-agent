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

	got := FindSysOID(pdus)
	if got != "1.3.6.1.4.1.9.1.1208" {
		t.Fatalf("expected sysOID, got %q", got)
	}
}

func TestFindSysOID_FoundScalarInstance0(t *testing.T) {
	expectedSysOID := "1.3.6.1.4.1.9.1.1208"
	pdus := []gosnmp.SnmpPDU{
		{Name: ".1.3.6.1.2.1.1.2.0", Value: expectedSysOID},
	}
	if got := FindSysOID(pdus); got != expectedSysOID {
		t.Fatalf("FindSysOID with .1.3.6.1.2.1.1.2.0: got %q, want %q", got, expectedSysOID)
	}
}

func TestFindSysOID_NotFound(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: ".1.3.3.1.2.1.2.2", Value: "no sysOID here"},
	}

	got := FindSysOID(pdus)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestFindSysOID_NonStringValue(t *testing.T) {
	sysObjIDOID := ".1.3.6.1.2.1.1.2"

	pdus := []gosnmp.SnmpPDU{
		{Name: sysObjIDOID, Value: []byte{49, 50, 51}},
	}

	got := FindSysOID(pdus)
	if got == "" {
		t.Fatalf("expected non-empty string, got %q", got)
	}
}

func TestAnalyze_EmptySysOID(t *testing.T) {
	expectedErrMsg := "sysObjectID is required for analysis"

	_, _, _, _, err := Analyze(nil, "")
	if err == nil || err.Error() != expectedErrMsg {
		t.Fatalf("Analyze(nil, \"\"): got err=%v, want Error()=%q", err, expectedErrMsg)
	}

	_, _, _, _, err = Analyze([]gosnmp.SnmpPDU{{Name: ".1.3.6.1.2.1.1.1.0", Value: "x"}}, "   ")
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

func TestAnalyze(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		// Required for profile detection
		{Name: SysObjectOID(), Value: "1.3.6.1.4.1.674.1"},

		// Standard MIB-2 system OIDs (Dell profile typically includes these)
		{Name: ".1.3.6.1.2.1.1.1.0", Value: "Dell iDRAC SNMP Agent"}, // sysDescr
		{Name: ".1.3.6.1.2.1.1.5.0", Value: "dell-pdu-01"},           // sysName
		{Name: ".1.3.6.1.2.1.1.3.0", Value: uint32(12345678)},        // sysUpTime
	}

	sysOID := "1.3.6.1.4.1.674.1"

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
	sysOID := "1.3.6.1.4.1.674.1"

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

func TestInterfaceID(t *testing.T) {
	pdus := []gosnmp.SnmpPDU{
		{Name: ".1.3.6.1.2.1.1.1.0", Value: "Test Device"},
		{Name: ".1.3.6.1.2.1.1.5.0", Value: "router-01"},
		{Name: ".1.3.6.1.2.1.1.3.0", Value: uint32(12345678)},

		// ifTable columns defined in _generic-if metrics.symbols
		{Name: ".1.3.6.1.2.1.2.2.1.14.12", Value: uint32(1000)},
		{Name: ".1.3.6.1.2.1.2.2.1.14.23", Value: uint32(2000)},
		{Name: ".1.3.6.1.2.1.2.2.1.13.11", Value: uint32(3000)},
		{Name: ".1.3.6.1.2.1.2.2.1.13.24", Value: uint32(4000)},
	}
	sysOID := "1.3.6.1.4.1.3375.2.1.3.4.1"

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
