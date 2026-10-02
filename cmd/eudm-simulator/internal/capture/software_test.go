// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
)

func TestSoftwareKeepsBackendEligibleVersionsWithoutLeakingOpaqueValues(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	const privateVersion = "2.3.4-preview.7+PRIVATE-SOFTWARE-SENTINEL"
	input := []software.Entry{
		{DisplayName: "Google Chrome", Version: "125.0.1", Source: "app", Status: "installed"},
		{DisplayName: "Private Application", Version: privateVersion, Source: "app", Status: "installed"},
		{DisplayName: "Other Private Application", Version: privateVersion, Source: "system_app", Status: "installed"},
		{DisplayName: "Homebrew package", Version: "4.4.28_1", Source: "homebrew", Status: "inactive (dependency)"},
		{DisplayName: "OS", Version: "26.0 (25A354)", Source: "os", Status: "installed"},
		{DisplayName: "Unversioned application", Source: "app", Status: "installed"},
	}
	before, _ := json.Marshal(input)
	clean := s.Software(input)
	if len(clean) != len(input) {
		t.Fatal("software snapshot lost entries")
	}
	opaque := regexp.MustCompile(`^software_version-[0-9a-f]{32}$`)
	for i, entry := range input {
		// softinv-reducer discards rows without name or version. Sanitization
		// must not turn an observed eligible application into a discarded row.
		if clean[i].DisplayName == "" || (clean[i].Version == "") != (entry.Version == "") {
			t.Fatal("sanitization changed software backend eligibility")
		}
	}
	if clean[0].Version != input[0].Version || clean[4].Version != input[4].Version || clean[5].Version != "" {
		t.Fatal("safe or absent software versions changed")
	}
	if !opaque.MatchString(clean[1].Version) || !opaque.MatchString(clean[3].Version) || clean[1].Version != clean[2].Version || clean[1].Version == clean[3].Version {
		t.Fatal("opaque software versions lost identity or resemble an invented release")
	}
	if !reflect.DeepEqual(clean, s.Software(input)) {
		t.Fatal("software identities changed between snapshots in one capture")
	}
	encoded, _ := json.Marshal(clean)
	for _, private := range []string{"PRIVATE-SOFTWARE-SENTINEL", "Private Application", "Homebrew package", "4.4.28_1"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private software fields survived sanitization")
		}
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("sanitization changed the original software snapshot")
	}
}

func TestSoftwarePreservesNativeKindsAndDeploymentStates(t *testing.T) {
	s, err := NewSanitizer()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"desktop", "msstore", "msi", "app", "system_app", "homebrew", "pkg", "macports", "mas", "os", "driver", "kext", "sysext"} {
		entry := software.Entry{DisplayName: "Google Chrome", Version: "1.2.3", Source: source, Status: "installed"}
		if got := s.Software([]software.Entry{entry})[0]; got.Source != source {
			t.Errorf("native software kind %q was erased", source)
		}
	}
	for _, status := range []string{
		"installed", "absent", "pending_install", "pending_removal", "uninstalling", "failed", "broken",
		"inactive", "imaged", "unknown", "installed (dependency)", "inactive (dependency)", "imaged (dependency)",
	} {
		entry := software.Entry{DisplayName: "package", Version: "1.2.3", Source: "homebrew", Status: status}
		if got := s.Software([]software.Entry{entry})[0]; got.Status != status {
			t.Errorf("native deployment state %q was erased", status)
		}
	}
	const private = "PRIVATE-SOFTWARE-SENTINEL"
	got := s.Software([]software.Entry{{DisplayName: "package", Version: "1.2.3", Source: private, Status: private}})[0]
	if got.Source != "" || got.Status != "" {
		t.Fatal("unknown classification strings escaped the finite allowlists")
	}
}
