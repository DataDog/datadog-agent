// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
)

func TestSoftwarePreservesCompleteSnapshotAndInstallationHistory(t *testing.T) {
	input := []software.Entry{
		{DisplayName: "Visual Studio Code", Version: "1.103.0-insider", Source: "app", Publisher: "Microsoft Corporation", Status: "installed", ProductCode: "com.microsoft.VSCode", UserSID: "alex", Is64Bit: true, InstallPaths: []string{"/Applications/Visual Studio Code.app"}, InstallDate: "2025-03-04T12:34:56.123Z"},
		{DisplayName: "Homebrew package", Version: "4.4.28_1", Source: "homebrew", Status: "inactive (dependency)", InstallPaths: []string{"/opt/homebrew/Cellar/package/4.4.28_1"}},
		{DisplayName: "OS", Version: "26.0 (25A354)", Source: "os", Status: "installed"},
		{DisplayName: "Unversioned application", Source: "future-native-source", Status: "future-native-state"},
		{DisplayName: "Windows application", Source: "msi", ProductCode: "{51ACABF3-0790-4205-BD14-67C233C169EF}", UserSID: "S-1-5-21-1234-5678-9012-1001", InstallPaths: []string{`C:\Program Files\Acme\app.exe`}},
	}
	out := NewNormalizer().Software(input)
	if !reflect.DeepEqual(input, out) {
		t.Fatal("software normalization changed observed values")
	}
	out[0].InstallPaths[0] = "changed"
	out[0].Version = "changed"
	if input[0].InstallPaths[0] != "/Applications/Visual Studio Code.app" || input[0].Version != "1.103.0-insider" {
		t.Fatal("software copy aliases the producer snapshot")
	}
}
