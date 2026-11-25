// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"slices"
	"testing"
	"time"

	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
)

func TestUsageAnchors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result types.Result
		pkg    ftypes.Package
		want   []string
	}{
		{
			name:   "OS package",
			result: types.Result{Class: types.ClassOSPkg, Type: ftypes.Debian},
			pkg:    ftypes.Package{InstalledFiles: []string{"/bin/bash", "/usr/share/doc/bash/copyright"}},
			want:   []string{"/bin/bash", "/usr/share/doc/bash/copyright"},
		},
		{
			name:   "wheel",
			result: types.Result{Class: types.ClassLangPkg, Type: ftypes.PythonPkg},
			pkg: ftypes.Package{InstalledFiles: []string{
				"/usr/lib/python3/dist-packages/yaml/__init__.py",
				"/usr/lib/python3/dist-packages/yaml/__pycache__/__init__.cpython-312.pyc",
				"/usr/lib/python3/dist-packages/_yaml.cpython-312-x86_64-linux-gnu.so",
				"/usr/lib/python3/dist-packages/PyYAML-6.0.1.dist-info/METADATA",
				"/usr/lib/python3/dist-packages/PyYAML-6.0.1.dist-info/RECORD",
				"/usr/lib/python3/dist-packages/yaml/py.typed",
				"/usr/local/bin/yamllint",
			}},
			want: []string{
				"/usr/lib/python3/dist-packages/yaml/__init__.py",
				"/usr/lib/python3/dist-packages/_yaml.cpython-312-x86_64-linux-gnu.so",
				"/usr/local/bin/yamllint",
			},
		},
		{
			name:   "Node.js package",
			result: types.Result{Class: types.ClassLangPkg, Type: ftypes.NodePkg},
			pkg:    ftypes.Package{FilePath: "app/node_modules/express/package.json"},
			want:   []string{"app/node_modules/express/package.json"},
		},
		{
			name:   "gem",
			result: types.Result{Class: types.ClassLangPkg, Type: ftypes.GemSpec},
			pkg:    ftypes.Package{FilePath: "usr/local/bundle/specifications/rake-13.0.6.gemspec"},
			want:   []string{"usr/local/bundle/specifications/rake-13.0.6.gemspec"},
		},
		{
			name:   "jar in a war",
			result: types.Result{Class: types.ClassLangPkg, Type: ftypes.Jar},
			pkg:    ftypes.Package{FilePath: "opt/app/app.war/WEB-INF/lib/guava.jar"},
			want:   []string{"opt/app/app.war"},
		},
		{
			name:   "Go module",
			result: types.Result{Class: types.ClassLangPkg, Type: ftypes.GoBinary, Target: "usr/local/bin/app"},
			pkg:    ftypes.Package{Name: "github.com/spf13/cobra"},
			want:   []string{"usr/local/bin/app"},
		},
		{
			name:   "Java runtime",
			result: types.Result{Class: types.ClassLangPkg, Type: ftypes.JavaExecutable},
			pkg:    ftypes.Package{FilePath: "opt/java/openjdk/bin/java"},
			want:   []string{"opt/java/openjdk/bin/java"},
		},
		{
			name:   "lock file",
			result: types.Result{Class: types.ClassLangPkg, Type: ftypes.Npm, Target: "app/package-lock.json"},
			pkg:    ftypes.Package{Name: "express"},
		},
		{
			name:   "node package without a path",
			result: types.Result{Class: types.ClassLangPkg, Type: ftypes.NodePkg},
			pkg:    ftypes.Package{Name: "express"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, usageAnchors(&tc.result, &tc.pkg))
		})
	}
}

func TestOuterArchive(t *testing.T) {
	for in, want := range map[string]string{
		"opt/app/app.jar":                         "opt/app/app.jar",
		"opt/app/app.jar/BOOT-INF/lib/guava.jar":  "opt/app/app.jar",
		"opt/app/app.war/WEB-INF/lib/guava.jar":   "opt/app/app.war",
		"opt/app/app.ear/lib/a.war/WEB-INF/b.jar": "opt/app/app.ear",
		"opt/app/lib.jar.d/x.jar":                 "opt/app/lib.jar.d/x.jar",
	} {
		assert.Equal(t, want, outerArchive(in), "outerArchive(%q)", in)
	}
}

// TestReportUsageIndex checks that the index of a report names the components
// of its SBOM, with bom-refs simplified or not, and that the run of a file
// credits the package that ships it.
func TestReportUsageIndex(t *testing.T) {
	report := &types.Report{
		ArtifactName: "root",
		ArtifactType: ftypes.TypeFilesystem,
		Metadata:     types.Metadata{OS: &ftypes.OS{Family: ftypes.Debian, Name: "12.7"}},
		Results: types.Results{
			{
				Target: "root (debian 12.7)",
				Class:  types.ClassOSPkg,
				Type:   ftypes.Debian,
				Packages: []ftypes.Package{
					{
						ID: "bash@5.2.15-2", Name: "bash", Version: "5.2.15-2",
						Identifier:     ftypes.PkgIdentifier{UID: "1"},
						InstalledFiles: []string{"/bin/bash"},
					},
					{
						ID: "coreutils@9.1-1", Name: "coreutils", Version: "9.1-1",
						Identifier:     ftypes.PkgIdentifier{UID: "2"},
						InstalledFiles: []string{"/usr/bin/ls"},
					},
				},
			},
			{
				Target: "Node.js",
				Class:  types.ClassLangPkg,
				Type:   ftypes.NodePkg,
				Packages: []ftypes.Package{{
					ID: "express@4.18.2", Name: "express", Version: "4.18.2",
					Identifier: ftypes.PkgIdentifier{UID: "3"},
					FilePath:   "app/node_modules/express/package.json",
				}},
			},
		},
	}

	start := time.Unix(1700000000, 0)
	for _, simplify := range []bool{false, true} {
		r, err := newReport("id", report, cyclonedx.NewMarshaler(""), reportOptions{usageIndex: true, simplifyBomRefs: simplify, scanStart: start})
		require.NoError(t, err)
		bom := r.ToCycloneDX()
		index := r.UsageIndex()
		require.NotNil(t, index)
		assert.Equal(t, bom.GetSerialNumber(), index.Serial())
		assert.True(t, index.ScanStart().Equal(start))

		names := make(map[string]string)
		for _, c := range bom.GetComponents() {
			names[c.GetBomRef()] = c.GetName()
		}
		var indexed []string
		for i := range index.Len() {
			name, ok := names[index.Component(i).BOMRef]
			require.True(t, ok, "bom-ref %q of the index outside the SBOM (simplified %v)", index.Component(i).BOMRef, simplify)
			indexed = append(indexed, name)
		}
		slices.Sort(indexed)
		assert.Equal(t, []string{"bash", "coreutils", "express"}, indexed)

		ran := &usage.Report{KeyKind: usage.KeyHost}
		for _, p := range []string{"/usr/bin/bash", "/app/node_modules/express/lib/router/index.js"} {
			ran.Keys = append(ran.Keys, usage.AppendKeys(nil, p)...)
		}
		slices.Sort(ran.Keys)
		for range ran.Keys {
			ran.LastSeen = append(ran.LastSeen, 1)
			ran.Flags = append(ran.Flags, 0)
		}
		require.NoError(t, ran.Validate())
		var used []string
		for i, u := range usage.Join(index, ran) {
			if u.LastSeen != 0 {
				used = append(used, names[index.Component(i).BOMRef])
			}
		}
		slices.Sort(used)
		assert.Equal(t, []string{"bash", "express"}, used, "simplified %v", simplify)
	}

	r, err := newReport("id", report, cyclonedx.NewMarshaler(""), reportOptions{})
	require.NoError(t, err)
	assert.Nil(t, r.UsageIndex())
	assert.Nil(t, (*Report)(nil).UsageIndex())
}
