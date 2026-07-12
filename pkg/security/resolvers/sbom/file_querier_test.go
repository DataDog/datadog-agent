// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import (
	"testing"

	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
)

// TestQueryFileUsrMerge checks that /bin and /usr/bin are treated as one tree on
// usr-merged layouts (where dpkg may record /bin/mount while execs resolve to
// /usr/bin/mount) and kept distinct otherwise.
func TestQueryFileUsrMerge(t *testing.T) {
	report := []sbomtypes.PackageWithInstalledFiles{
		{Package: sbomtypes.Package{Name: "mount"}, InstalledFiles: []string{"/bin/mount"}},
		{Package: sbomtypes.Package{Name: "util-linux"}, InstalledFiles: []string{"/bin/su"}},
		{Package: sbomtypes.Package{Name: "passwd"}, InstalledFiles: []string{"/usr/bin/passwd"}},
		{Package: sbomtypes.Package{Name: "coreutils"}, InstalledFiles: []string{"/usr/bin/cat"}},
	}

	backing := make([]sbomtypes.Package, len(report))
	for i := range report {
		backing[i] = report[i].Package
	}

	merged := newFileQuerier(report, backing, true)
	for _, tc := range []struct {
		name    string
		query   string
		wantPkg string
	}{
		{"/bin recorded, exec'd as /usr/bin", "/usr/bin/mount", "mount"},
		{"su resolves to util-linux", "/usr/bin/su", "util-linux"},
		{"/usr/bin recorded, queried as /bin", "/bin/cat", "coreutils"},
		{"direct hit", "/usr/bin/passwd", "passwd"},
		{"unknown file", "/usr/bin/does-not-exist", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ""
			if pkg := merged.queryFile(tc.query); pkg != nil {
				got = pkg.Name
			}
			if got != tc.wantPkg {
				t.Errorf("queryFile(%q) = %q, want %q", tc.query, got, tc.wantPkg)
			}
		})
	}

	// Without usr-merge, /bin and /usr/bin are distinct trees: an unmatched
	// /usr/bin/mount must not be cross-attributed to the /bin/mount package.
	plain := newFileQuerier(report, backing, false)
	if pkg := plain.queryFile("/usr/bin/mount"); pkg != nil {
		t.Errorf("queryFile(/usr/bin/mount) attributed to %q on a non-usr-merged layout, want no match", pkg.Name)
	}
}
