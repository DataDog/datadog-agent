// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/google/go-cmp/cmp"
	"github.com/package-url/packageurl-go"
	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
)

// pkgWithPurl returns a package carrying the source-report identifiers the real
// Trivy applier calculates before SBOM encoding.
func pkgWithPurl(name, version string, qualifiers ...string) ftypes.Package {
	var qs packageurl.Qualifiers
	for i := 0; i+1 < len(qualifiers); i += 2 {
		qs = append(qs, packageurl.Qualifier{Key: qualifiers[i], Value: qualifiers[i+1]})
	}
	purl := packageurl.NewPackageURL("generic", "", name, version, qs, "")
	return ftypes.Package{
		Name:       name,
		Version:    version,
		Identifier: ftypes.PkgIdentifier{UID: purl.String(), PURL: purl},
	}
}

func buildTestUsageIndex(report *types.Report, scan usage.ScanID, generation uint64, root string) *usage.Index {
	refs := make(map[string]string)
	for _, result := range report.Results {
		for _, pkg := range result.Packages {
			if pkg.Identifier.UID != "" {
				refs[pkg.Identifier.UID] = "ref:" + pkg.Identifier.UID
			}
		}
	}
	return BuildUsageIndex(report, refs, "urn:uuid:test-index", scan, generation, root)
}

// names returns the component names the given path resolves to, in the order
// Lookup returns them.
func names(idx *usage.Index, path string) []string {
	var out []string
	for _, ref := range idx.Lookup(murmur3.StringSum64(path)) {
		out = append(out, idx.Component(ref).Name)
	}
	return out
}

func TestBuildUsageIndexSources(t *testing.T) {
	gzip := pkgWithPurl("gzip", "1.12")
	gzip.InstalledFiles = []string{"/usr/bin/gzip", "/usr/share/man/man1/gzip.1.gz"}

	jar := pkgWithPurl("commons-io", "2.11.0")
	jar.FilePath = "/opt/app/lib/commons-io-2.11.0.jar"

	// A Go binary is one file holding every module it reports, and its main
	// module is the answer a caller wanting one component should get.
	main := pkgWithPurl("example.com/app", "1.0.0")
	main.Relationship = ftypes.RelationshipRoot
	dep := pkgWithPurl("example.com/dep", "0.4.0")

	// A lock file names packages installed elsewhere, so its target says the
	// package manager ran rather than that anything was used.
	locked := pkgWithPurl("left-pad", "1.3.0")

	report := &types.Report{Results: types.Results{
		{Target: "rhel", Class: types.ClassOSPkg, Type: ftypes.RedHat, Packages: []ftypes.Package{gzip}},
		{Target: "Java", Class: types.ClassLangPkg, Type: ftypes.Jar, Packages: []ftypes.Package{jar}},
		{Target: "/usr/bin/app", Class: types.ClassLangPkg, Type: ftypes.GoBinary, Packages: []ftypes.Package{dep, main}},
		{Target: "package-lock.json", Class: types.ClassLangPkg, Type: ftypes.Npm, Packages: []ftypes.Package{locked}},
	}}

	idx := buildTestUsageIndex(report, "sha256:image", 3, "")

	if idx.Generation != 3 || idx.Scan != "sha256:image" || idx.Status != usage.Ready {
		t.Errorf("index header = {%s %d %v}, want {sha256:image 3 Ready}", idx.Scan, idx.Generation, idx.Status)
	}
	// Every package stays a component, including the lock-file entry, so the
	// stamp can tell "no file to watch" from "watched and idle".
	if got, want := len(idx.Components), 5; got != want {
		t.Fatalf("components = %d, want %d", got, want)
	}

	tests := []struct {
		name string
		path string
		want []string
	}{
		{"os package installed file", "/usr/bin/gzip", []string{"gzip"}},
		{"artifact anchor", "/opt/app/lib/commons-io-2.11.0.jar", []string{"commons-io"}},
		{"binary target names every module, main module first", "/usr/bin/app", []string{"example.com/app", "example.com/dep"}},
		{"lock file target is not indexed", "/package-lock.json", nil},
		{"unrelated path", "/etc/hosts", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, names(idx, tt.path)); diff != "" {
				t.Errorf("lookup mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildUsageIndexUsrMerged(t *testing.T) {
	// A merged layout records one spelling; the kernel may resolve the other.
	merged := pkgWithPurl("coreutils", "9.1")
	merged.InstalledFiles = []string{"/bin/cat"}

	idx := buildTestUsageIndex(&types.Report{Results: types.Results{
		{Class: types.ClassOSPkg, Type: ftypes.Debian, Packages: []ftypes.Package{merged}},
	}}, "sha256:image", 1, "")

	for _, path := range []string{"/bin/cat", "/usr/bin/cat"} {
		if diff := cmp.Diff([]string{"coreutils"}, names(idx, path)); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", path, diff)
		}
	}
}

func TestBuildUsageIndexSplitUsrKeepsOwners(t *testing.T) {
	// A split layout has both spellings as real, separately owned files, so
	// aliasing one onto the other would attribute a file to the wrong package.
	binSh := pkgWithPurl("dash", "0.5.12")
	binSh.InstalledFiles = []string{"/bin/sh"}
	usrBinSh := pkgWithPurl("bash", "5.2")
	usrBinSh.InstalledFiles = []string{"/usr/bin/sh"}

	idx := buildTestUsageIndex(&types.Report{Results: types.Results{
		{Class: types.ClassOSPkg, Type: ftypes.Debian, Packages: []ftypes.Package{binSh, usrBinSh}},
	}}, "sha256:image", 1, "")

	if diff := cmp.Diff([]string{"dash"}, names(idx, "/bin/sh")); diff != "" {
		t.Errorf("/bin/sh mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"bash"}, names(idx, "/usr/bin/sh")); diff != "" {
		t.Errorf("/usr/bin/sh mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildUsageIndexMultiarch(t *testing.T) {
	// The two dpkg entries of a multiarch install share a name and version; the
	// PURL qualifier preserves their package-coordinate distinction, while each
	// final BOM occurrence still has its own BOM ref.
	amd64 := pkgWithPurl("libc6", "2.36", "arch", "amd64")
	amd64.InstalledFiles = []string{"/usr/lib/x86_64-linux-gnu/libc.so.6"}
	i386 := pkgWithPurl("libc6", "2.36", "arch", "i386")
	i386.InstalledFiles = []string{"/usr/lib/i386-linux-gnu/libc.so.6"}

	idx := buildTestUsageIndex(&types.Report{Results: types.Results{
		{Class: types.ClassOSPkg, Type: ftypes.Debian, Packages: []ftypes.Package{amd64, i386}},
	}}, "sha256:image", 1, "")

	purls := map[string]bool{}
	for _, comp := range idx.Components {
		purls[comp.Purl] = true
	}
	if len(purls) != 2 {
		t.Errorf("distinct purls = %d, want 2: %v", len(purls), purls)
	}
}

func TestBuildUsageIndexAnchorsPURLLessComponent(t *testing.T) {
	local := ftypes.Package{Name: "local", Version: "0.0.1", Identifier: ftypes.PkgIdentifier{UID: "local-uid"}}
	idx := buildTestUsageIndex(&types.Report{Results: types.Results{
		{Class: types.ClassLangPkg, Type: ftypes.GoBinary, Target: "/usr/bin/local", Packages: []ftypes.Package{local}},
	}}, "sha256:image", 1, "")

	if got, want := len(idx.Components), 1; got != want {
		t.Fatalf("components = %d, want %d", got, want)
	}
	if idx.Components[0].Purl != "" {
		t.Errorf("purl = %q, want empty", idx.Components[0].Purl)
	}
	if idx.Components[0].BOMRef == "" {
		t.Error("PURL-less component has no BOM ref")
	}
	if got := usage.NewTable(idx).Anchored(); got != 1 {
		t.Errorf("anchored = %d, want 1", got)
	}
}

func TestBuildUsageIndexPythonRecord(t *testing.T) {
	// CPython opens the code files on import, never the metadata Trivy anchors
	// the wheel to, so the RECORD beside it is what makes a wheel measurable.
	root := t.TempDir()
	distInfo := filepath.Join(root, "usr/lib/python3.11/site-packages/requests-2.31.0.dist-info")
	if err := os.MkdirAll(distInfo, 0o755); err != nil {
		t.Fatal(err)
	}
	record := "requests/__init__.py,sha256=abc,123\nrequests/api.py,sha256=def,456\n\"odd,name.py\",,\n"
	if err := os.WriteFile(filepath.Join(distInfo, "RECORD"), []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}

	wheel := pkgWithPurl("requests", "2.31.0")
	wheel.FilePath = "/usr/lib/python3.11/site-packages/requests-2.31.0.dist-info/METADATA"
	report := &types.Report{Results: types.Results{
		{Class: types.ClassLangPkg, Type: ftypes.PythonPkg, Target: "Python", Packages: []ftypes.Package{wheel}},
	}}

	withRoot := buildTestUsageIndex(report, "sha256:image", 1, root)
	for _, path := range []string{
		"/usr/lib/python3.11/site-packages/requests/__init__.py",
		"/usr/lib/python3.11/site-packages/requests/api.py",
		"/usr/lib/python3.11/site-packages/requests-2.31.0.dist-info/METADATA",
	} {
		if diff := cmp.Diff([]string{"requests"}, names(withRoot, path)); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", path, diff)
		}
	}

	// A scan of an image has no filesystem to read, so the wheel keeps only the
	// anchor Trivy gave it.
	withoutRoot := buildTestUsageIndex(report, "sha256:image", 1, "")
	if got := names(withoutRoot, "/usr/lib/python3.11/site-packages/requests/__init__.py"); got != nil {
		t.Errorf("code file resolved without a root: %v", got)
	}
}

// TestBuildUsageIndexCarriesPaths covers the table a consumer needs when it
// builds kernel-side filters from the file names rather than matching hashes.
// Without them the CWS policy generator has nothing to write a macro from.
func TestBuildUsageIndexCarriesPaths(t *testing.T) {
	gzip := pkgWithPurl("gzip", "1.12")
	gzip.InstalledFiles = []string{"/usr/bin/gzip"}

	idx := buildTestUsageIndex(&types.Report{Results: types.Results{
		{Class: types.ClassOSPkg, Type: ftypes.RedHat, Packages: []ftypes.Package{gzip}},
	}}, "image:sha256:x", 1, "")

	if len(idx.Paths) != len(idx.Hashes) || len(idx.Paths) != len(idx.Refs) {
		t.Fatalf("paths/hashes/refs lengths differ: %d/%d/%d", len(idx.Paths), len(idx.Hashes), len(idx.Refs))
	}
	if !slices.Contains(idx.Paths, "/usr/bin/gzip") {
		t.Errorf("paths = %v, want it to carry /usr/bin/gzip", idx.Paths)
	}
	// Each path sits at the position of its own hash, which is what lets a
	// consumer pair the two.
	for i, path := range idx.Paths {
		if got := murmur3.StringSum64(path); got != idx.Hashes[i] {
			t.Errorf("paths[%d]=%q hashes to %d, want %d", i, path, got, idx.Hashes[i])
		}
	}
}

func TestFillDistinguishesMultiMappingFromHashCollision(t *testing.T) {
	constantHash := func(string) uint64 { return 7 }

	t.Run("one path may name several components", func(t *testing.T) {
		idx := &usage.Index{Scan: "image:x"}
		fillWithHasher(idx, []indexEntry{
			{path: "/usr/bin/app", ref: 0},
			{path: "/usr/bin/app", ref: 1},
		}, constantHash)
		if diff := cmp.Diff([]uint64{7, 7}, idx.Hashes); diff != "" {
			t.Errorf("hashes mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]uint32{0, 1}, idx.Refs); diff != "" {
			t.Errorf("refs mismatch (-want +got):\n%s", diff)
		}
		if idx.HashCollisions != 0 {
			t.Errorf("hash collisions = %d, want 0", idx.HashCollisions)
		}
	})

	t.Run("distinct colliding paths are all dropped", func(t *testing.T) {
		idx := &usage.Index{Scan: "image:x"}
		fillWithHasher(idx, []indexEntry{
			{path: "/usr/bin/a", ref: 0},
			{path: "/usr/bin/b", ref: 1},
		}, constantHash)
		if len(idx.Hashes) != 0 || len(idx.Refs) != 0 || len(idx.Paths) != 0 {
			t.Errorf("colliding entries survived: hashes=%v refs=%v paths=%v", idx.Hashes, idx.Refs, idx.Paths)
		}
		if idx.HashCollisions != 1 {
			t.Errorf("hash collisions = %d, want 1", idx.HashCollisions)
		}
	})
}

func runtimeProperty(comp *cyclonedx_v1_4.Component, name string) string {
	for _, property := range comp.GetProperties() {
		if property.GetName() == name {
			return property.GetValue()
		}
	}
	return ""
}

// TestIndexBOMRefsMatchTheFinalBOM exercises the real Trivy encoder, including
// its duplicate-PURL and PURL-less BOM-ref rules, then follows one occurrence
// through the Agent's optional ref simplification, usage index, and stamp.
func TestIndexBOMRefsMatchTheFinalBOM(t *testing.T) {
	first := pkgWithPurl("actionpack", "7.0.0")
	first.Identifier.UID = "actionpack:first-lockfile"
	first.FilePath = "/app/first/actionpack.gemspec"
	second := first
	second.Identifier.UID = "actionpack:second-lockfile"
	second.FilePath = "/app/second/actionpack.gemspec"
	local := ftypes.Package{
		Name:       "local",
		Version:    "0.0.1",
		FilePath:   "/app/local.pkg",
		Identifier: ftypes.PkgIdentifier{UID: "local:purl-less"},
	}
	report := &types.Report{
		ArtifactName: "sha256:x",
		ArtifactType: ftypes.TypeContainerImage,
		Results: types.Results{{
			Target: "/app", Class: types.ClassLangPkg, Type: ftypes.GemSpec,
			Packages: []ftypes.Package{first, second, local},
		}},
	}

	for _, simplify := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserved", true: "simplified"}[simplify], func(t *testing.T) {
			built, err := newReport("sha256:x", report, cyclonedx.NewMarshaler(""), reportOptions{
				dependencies: true, simplifyBomRefs: simplify,
			})
			if err != nil {
				t.Fatal(err)
			}
			idx := BuildUsageIndex(report, built.componentRefs, built.indexID, "image:sha256:x", 1, "")
			if idx.IndexID == "" || idx.IndexID != built.ToCycloneDX().GetSerialNumber() {
				t.Errorf("index ID = %q, BOM serial = %q", idx.IndexID, built.ToCycloneDX().GetSerialNumber())
			}
			if idx.UnmappedComponents != 0 {
				t.Errorf("unmapped components = %d, want 0", idx.UnmappedComponents)
			}
			if got := len(idx.Components); got != 3 {
				t.Fatalf("index components = %d, want 3", got)
			}
			if idx.Components[0].Purl != idx.Components[1].Purl {
				t.Fatalf("duplicate test PURLs differ: %q and %q", idx.Components[0].Purl, idx.Components[1].Purl)
			}
			if idx.Components[0].BOMRef == idx.Components[1].BOMRef {
				t.Fatalf("duplicate-PURL occurrences share BOM ref %q", idx.Components[0].BOMRef)
			}
			if idx.Components[2].Purl != "" || idx.Components[2].BOMRef == "" {
				t.Errorf("PURL-less component identity = {%q %q}", idx.Components[2].Purl, idx.Components[2].BOMRef)
			}

			inBOM := make(map[string]int)
			for _, comp := range built.ToCycloneDX().GetComponents() {
				inBOM[comp.GetBomRef()]++
			}
			for _, comp := range idx.Components {
				if count := inBOM[comp.BOMRef]; count != 1 {
					t.Errorf("index BOM ref %q occurs %d times in final BOM, want once", comp.BOMRef, count)
				}
			}

			table := usage.NewTable(idx)
			result := table.Apply(&usage.Report{
				Scan: idx.Scan, Generation: idx.Generation, IndexID: idx.IndexID,
				Usage: []usage.Usage{{Ref: 0, LastSeen: time.Unix(1700000000, 0)}},
			})
			if !result.Applied {
				t.Fatal("usage report was rejected")
			}
			stamped := table.Stamp(built.ToCycloneDX())
			byRef := make(map[string]*cyclonedx_v1_4.Component)
			for _, comp := range stamped.GetComponents() {
				byRef[comp.GetBomRef()] = comp
			}
			if got := runtimeProperty(byRef[idx.Components[0].BOMRef], usage.LastSeenRunningProperty); got != "1700000000" {
				t.Errorf("observed occurrence timestamp = %q, want 1700000000", got)
			}
			if got := runtimeProperty(byRef[idx.Components[1].BOMRef], usage.LastSeenRunningProperty); got != "0" {
				t.Errorf("other duplicate-PURL occurrence timestamp = %q, want 0", got)
			}
		})
	}
}

func TestBuildUsageIndexLeavesAmbiguousUIDUnmapped(t *testing.T) {
	first := pkgWithPurl("duplicate", "1.0.0")
	first.Identifier.UID = "same-uid"
	first.FilePath = "/app/first"
	second := first
	second.FilePath = "/app/second"
	report := &types.Report{
		ArtifactName: "x",
		ArtifactType: ftypes.TypeContainerImage,
		Results: types.Results{{
			Target: "/app", Class: types.ClassLangPkg, Type: ftypes.GemSpec,
			Packages: []ftypes.Package{first, second},
		}},
	}
	built, err := newReport("x", report, cyclonedx.NewMarshaler(""), reportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	idx := BuildUsageIndex(report, built.componentRefs, built.indexID, "image:x", 1, "")
	if idx.UnmappedComponents != 2 || idx.Components[0].BOMRef != "" || idx.Components[1].BOMRef != "" {
		t.Errorf("unmapped identities = count %d refs %q/%q, want 2 and empty", idx.UnmappedComponents, idx.Components[0].BOMRef, idx.Components[1].BOMRef)
	}
	if got := usage.NewTable(idx).Anchored(); got != 0 {
		t.Errorf("ambiguous component anchored = %d, want 0", got)
	}
	table := usage.NewTable(idx)
	result := table.Apply(&usage.Report{
		Scan: idx.Scan, Generation: idx.Generation, IndexID: idx.IndexID,
		Usage: []usage.Usage{{Ref: 0}, {Ref: 1}},
	})
	if !result.Applied || len(table.Seen()) != 0 {
		t.Errorf("unmapped indexed refs poisoned report: result=%#v seen=%v", result, table.Seen())
	}
}
