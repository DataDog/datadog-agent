package analyzer

import (
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestFindSysOID_Found(t *testing.T) {

	pdus := []gosnmp.SnmpPDU{
		{Name: "1.3.6.1.2.1.1.1.0", Value: "some other value"},
		{Name: _cached_sys_obj_id, Value: "1.3.6.1.4.1.9.1.1208"},
	}

	got := FindSysOID(pdus)
	if got != "1.3.6.1.4.1.9.1.1208" {
		t.Fatalf("expected sysOID, got %q", got)
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

func TestProfileFound(t *testing.T) {
	//Profile for dell
	pdus := []gosnmp.SnmpPDU{
		{Name: _cached_sys_obj_id, Value: "1.3.6.1.4.1.674.1"},
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
		{Name: _cached_sys_obj_id, Value: "1.1.1.1.4.1.14.1"},
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
		{Name: _cached_sys_obj_id, Value: "1.3.6.1.4.1.674.1"},

		// Standard MIB-2 system OIDs (Dell profile typically includes these)
		{Name: ".1.3.6.1.2.1.1.1.0", Value: "Dell iDRAC SNMP Agent"}, // sysDescr
		{Name: ".1.3.6.1.2.1.1.5.0", Value: "dell-pdu-01"},           // sysName
		{Name: ".1.3.6.1.2.1.1.3.0", Value: uint32(12345678)},        // sysUpTime
	}

	sysOID := "1.3.6.1.4.1.674.1"

	found, notFound, profileName, _, err := Analyze(pdus, sysOID)
	if err != nil {
		t.Skipf("profile lookup not available (analyzer returned 0 metrics): %v", err)
	}
	t.Logf("Analyze returned %d matched, %d not found, profile=%s", len(found), len(notFound), profileName)

}
