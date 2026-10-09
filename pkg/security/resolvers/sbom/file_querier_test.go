// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sbom

import (
	"fmt"
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

// TestQueryFileFirstOwnerWins checks that a path two packages list, as rpm does
// for a directory they share, resolves to the first of them in the report, as
// it did when the index was walked in report order.
func TestQueryFileFirstOwnerWins(t *testing.T) {
	report := []sbomtypes.PackageWithInstalledFiles{
		{Package: sbomtypes.Package{Name: "filesystem"}, InstalledFiles: []string{"/usr/share/doc", "/usr/bin"}},
		{Package: sbomtypes.Package{Name: "bash"}, InstalledFiles: []string{"/usr/bin/bash", "/usr/share/doc", "/usr/bin"}},
		{Package: sbomtypes.Package{Name: "coreutils"}, InstalledFiles: []string{"/usr/bin", "/usr/bin/cat"}},
	}

	backing := make([]sbomtypes.Package, len(report))
	for i := range report {
		backing[i] = report[i].Package
	}

	fq := newFileQuerier(report, backing, false)
	for _, tc := range []struct {
		query   string
		wantPkg string
	}{
		{"/usr/share/doc", "filesystem"},
		{"/usr/bin", "filesystem"},
		{"/usr/bin/bash", "bash"},
		{"/usr/bin/cat", "coreutils"},
	} {
		got := ""
		if pkg := fq.queryFile(tc.query); pkg != nil {
			got = pkg.Name
		}
		if got != tc.wantPkg {
			t.Errorf("queryFile(%q) = %q, want %q", tc.query, got, tc.wantPkg)
		}
	}
}

// hostReport returns the installed files of a host the size of an Ubuntu 24.04
// machine, 2,171 packages listing 250,069 files, and the packages backing them.
func hostReport() ([]sbomtypes.PackageWithInstalledFiles, []sbomtypes.Package) {
	const packages, files = 2171, 250069
	dirs := []string{"/usr/bin", "/usr/lib", "/usr/sbin", "/usr/share"}

	report := make([]sbomtypes.PackageWithInstalledFiles, packages)
	for i := range report {
		report[i].Package = sbomtypes.Package{Name: fmt.Sprintf("pkg%d", i)}
	}
	for f := range files {
		i := f % packages
		report[i].InstalledFiles = append(report[i].InstalledFiles, fmt.Sprintf("%s/pkg%d/file%d", dirs[f%len(dirs)], i, f))
	}

	backing := make([]sbomtypes.Package, len(report))
	for i := range report {
		backing[i] = report[i].Package
	}
	return report, backing
}

// BenchmarkQueryFile measures a lookup in the index of a host on a merged /usr:
// a file of a package, a missing file, and a missing file under /bin, which
// takes a second lookup under /usr/bin. The missing files rotate, as the
// accesses of a host do.
func BenchmarkQueryFile(b *testing.B) {
	report, backing := hostReport()
	fq := newFileQuerier(report, backing, true)

	for _, bm := range []struct {
		name  string
		paths []string
	}{
		{"hit", []string{report[len(report)/2].InstalledFiles[0]}},
		{"miss", []string{"/usr/share/missing0", "/usr/share/missing1", "/usr/share/missing2", "/usr/share/missing3"}},
		{"usr-merged-miss", []string{"/bin/missing0", "/bin/missing1", "/bin/missing2", "/bin/missing3"}},
	} {
		b.Run(bm.name, func(b *testing.B) {
			i := 0
			for b.Loop() {
				fq.queryFile(bm.paths[i%len(bm.paths)])
				i++
			}
		})
	}
}

// BenchmarkNewFileQuerier measures the indexing of the files of a host.
func BenchmarkNewFileQuerier(b *testing.B) {
	report, backing := hostReport()
	for b.Loop() {
		newFileQuerier(report, backing, true)
	}
}

// TestQueryFileMissesReplacedFile checks that the " (deleted)" name procfs gives
// a file an upgrade replaced under a running process resolves to no package.
func TestQueryFileMissesReplacedFile(t *testing.T) {
	report := []sbomtypes.PackageWithInstalledFiles{
		{Package: sbomtypes.Package{Name: "gzip"}, InstalledFiles: []string{"/usr/bin/gzip"}},
	}
	fq := newFileQuerier(report, []sbomtypes.Package{report[0].Package}, true)

	if pkg := fq.queryFile("/usr/bin/gzip"); pkg == nil || pkg.Name != "gzip" {
		t.Errorf("/usr/bin/gzip resolves to %+v, want gzip", pkg)
	}
	if pkg := fq.queryFile("/usr/bin/gzip (deleted)"); pkg != nil {
		t.Errorf("the replaced gzip resolves to %+v", pkg)
	}
}
