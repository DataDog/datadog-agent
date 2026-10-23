// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy && containerd

// Package trivy holds the scan components
package trivy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/ddtrivy"
)

// TestExtractLayersFromOverlayFSMounts checks if the function correctly extracts layer paths from Mount options.
func TestExtractLayersFromOverlayFSMounts(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mounts []mount.Mount
		want   []string
	}{
		{
			name:   "No mounts",
			mounts: []mount.Mount{},
		},
		{
			name:   "Single upperdir",
			mounts: []mount.Mount{{Options: []string{"someoption=somevalue", "upperdir=/path/to/upper"}}},
			want:   []string{"/path/to/upper"},
		},
		{
			name:   "Single lowerdir",
			mounts: []mount.Mount{{Options: []string{"someoption=somevalue", "lowerdir=/path/to/lower"}}},
			want:   []string{"/path/to/lower"},
		},
		{
			name:   "Multiple lowerdir",
			mounts: []mount.Mount{{Options: []string{"someoption=somevalue", "lowerdir=/path/to/lower1:/path/to/lower2"}}},
			want:   []string{"/path/to/lower1", "/path/to/lower2"},
		},
		{
			name:   "Multiple options",
			mounts: []mount.Mount{{Options: []string{"someoption=somevalue", "upperdir=/path/to/upper", "lowerdir=/path/to/lower1:/path/to/lower2"}}},
			want:   []string{"/path/to/upper", "/path/to/lower1", "/path/to/lower2"},
		},
		{
			name: "Multiple mounts",
			mounts: []mount.Mount{
				{Options: []string{"someoption=somevalue", "upperdir=/path/to/upper1"}},
				{Options: []string{"someoption=somevalue", "lowerdir=/path/to/lower1:/path/to/lower2"}},
			},
			want: []string{"/path/to/upper1", "/path/to/lower1", "/path/to/lower2"},
		},
		{
			// A single-layer image is exposed by containerd as a single bind mount
			// (no lowerdir/upperdir); its only layer is the mount source.
			name:   "Single-layer bind mount",
			mounts: []mount.Mount{{Type: "bind", Source: "/path/to/snapshots/132/fs", Options: []string{"ro", "rbind"}}},
			want:   []string{"/path/to/snapshots/132/fs"},
		},
		{
			// Overlay options take precedence; the mount source is not a layer path.
			name:   "Overlay mount source ignored",
			mounts: []mount.Mount{{Type: "overlay", Source: "overlay", Options: []string{"lowerdir=/path/to/lower"}}},
			want:   []string{"/path/to/lower"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractLayersFromOverlayFSMounts(tt.mounts))
		})
	}
}

// TestScanFilesystemImageCreated covers the use_mount path, where the caller
// supplies the build time of the image it mounted.
func TestScanFilesystemImageCreated(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "etc", "os-release"), []byte("ID=alpine\nVERSION_ID=3.20.0\n"), 0o644))

	tests := []struct {
		name    string
		created time.Time
		want    []string
	}{
		{
			name:    "offset is normalized to UTC",
			created: time.Date(2024, time.November, 7, 5, 26, 15, 123456789, time.FixedZone("UTC+5", 5*60*60)),
			want:    []string{"2024-11-07T00:26:15.123456789Z"},
		},
		{
			name: "config omits created",
		},
	}
	c := NewCollectorForCLI()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := c.scanFilesystem(t.Context(), root, sbom.ScanOptions{Analyzers: []string{OSAnalyzers}}, false, tt.created)
			require.NoError(t, err)

			var got []string
			for _, p := range report.ToCycloneDX().GetMetadata().GetComponent().GetProperties() {
				if p.GetName() == imageCreatedPropertyKey {
					got = append(got, p.GetValue())
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestScanFilesystemLicenseFile checks that a Python package whose metadata
// names only a License-File keeps the file reference out of its licenses.
func TestScanFilesystemLicenseFile(t *testing.T) {
	root := t.TempDir()
	distInfo := filepath.Join(root, "usr", "lib", "python3", "dist-packages", "example-1.0.0.dist-info")
	require.NoError(t, os.MkdirAll(distInfo, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(distInfo, "METADATA"), []byte("Metadata-Version: 2.1\nName: example\nVersion: 1.0.0\nLicense-File: LICENSE\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(distInfo, "LICENSE"), []byte("MIT License\n"), 0o644))

	c := NewCollectorForCLI()
	report, err := c.scanFilesystem(t.Context(), root, sbom.ScanOptions{Analyzers: []string{LanguagesAnalyzers}}, false, time.Time{})
	require.NoError(t, err)

	found := false
	for _, comp := range report.ToCycloneDX().GetComponents() {
		if !strings.HasPrefix(comp.GetPurl(), "pkg:pypi/example@") {
			continue
		}
		found = true
		for _, l := range comp.GetLicenses() {
			for _, s := range []string{l.GetLicense().GetId(), l.GetLicense().GetName(), l.GetExpression()} {
				assert.Falsef(t, strings.HasPrefix(s, "file:"), "license %q names the license file", s)
			}
		}
	}
	assert.True(t, found, "the SBOM lacks the example package")
}

func TestGetDefaultArtifactOption(t *testing.T) {
	tests := []struct {
		name      string
		analyzers []string
		want      artifact.Option
	}{
		{
			name:      "os",
			analyzers: []string{OSAnalyzers},
			want:      ddtrivy.TrivyOptionsOS(1),
		},
		{
			name:      "os and languages",
			analyzers: []string{OSAnalyzers, LanguagesAnalyzers},
			want:      ddtrivy.TrivyOptionsAll(1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDefaultArtifactOption(sbom.ScanOptions{Analyzers: tt.analyzers})
			assert.ElementsMatch(t, tt.want.DisabledAnalyzers, got.DisabledAnalyzers)
			assert.Equal(t, tt.want.AnalyzerTimeout, got.AnalyzerTimeout)
			assert.Equal(t, tt.want.PostAnalyzerTimeout, got.PostAnalyzerTimeout)
			assert.True(t, got.OfflineJar)
		})
	}

	t.Run("os with additional directories", func(t *testing.T) {
		got := getDefaultArtifactOption(sbom.ScanOptions{Analyzers: []string{OSAnalyzers}, AdditionalDirs: []string{"srv/app/**"}})
		assert.Empty(t, got.WalkerOption.OnlyDirs)
	})

	t.Run("os and languages with additional directories", func(t *testing.T) {
		got := getDefaultArtifactOption(sbom.ScanOptions{Analyzers: []string{OSAnalyzers, LanguagesAnalyzers}, AdditionalDirs: []string{"srv/app/**"}})
		assert.Contains(t, got.WalkerOption.OnlyDirs, "srv/app/**")
	})
}

func TestScanWithOSFallback(t *testing.T) {
	osReport := &types.Report{ArtifactName: "os"}
	appReport := &types.Report{ArtifactName: "app"}
	errScan := errors.New("scan failed")

	// Each scan step plays one call to scan.
	finish := func(report *types.Report) func(context.Context) (*types.Report, error) {
		return func(context.Context) (*types.Report, error) { return report, nil }
	}
	fail := func(err error) func(context.Context) (*types.Report, error) {
		return func(context.Context) (*types.Report, error) { return nil, err }
	}
	// block runs until the context is done.
	block := func(ctx context.Context) (*types.Report, error) {
		<-ctx.Done()
		return nil, fmt.Errorf("walk dir error: %w", ctx.Err())
	}
	// blockFail runs until the context is done and fails with an error
	// that wraps no context error.
	blockFail := func(ctx context.Context) (*types.Report, error) {
		<-ctx.Done()
		return nil, errScan
	}
	// overrun runs past the wall, ignoring the context.
	overrun := func(context.Context) (*types.Report, error) {
		time.Sleep(13 * time.Minute)
		return nil, errScan
	}

	type call struct {
		left      time.Duration // time to the deadline as the call starts, 0 without one
		analyzers []string      // analyzers the call's options come from
	}
	app := []string{OSAnalyzers, LanguagesAnalyzers}
	osOnly := []string{OSAnalyzers}
	tests := []struct {
		name         string
		analyzers    []string
		timeout      time.Duration // 0 for no deadline
		cancel       bool          // cancel the context in the first call
		scans        []func(context.Context) (*types.Report, error)
		want         *types.Report
		wantFallback bool
		wantErr      error
		wantCalls    []call
	}{
		{
			name:      "os only",
			analyzers: osOnly,
			timeout:   10 * time.Minute,
			scans:     []func(context.Context) (*types.Report, error){finish(osReport)},
			want:      osReport,
			wantCalls: []call{{left: 10 * time.Minute, analyzers: osOnly}},
		},
		{
			name:      "os only without a deadline",
			analyzers: osOnly,
			scans:     []func(context.Context) (*types.Report, error){finish(osReport)},
			want:      osReport,
			wantCalls: []call{{analyzers: osOnly}},
		},
		{
			name:      "no deadline",
			analyzers: app,
			scans:     []func(context.Context) (*types.Report, error){finish(appReport)},
			want:      appReport,
			wantCalls: []call{{analyzers: app}},
		},
		{
			name:      "application scan finishes",
			analyzers: app,
			timeout:   12 * time.Minute,
			scans:     []func(context.Context) (*types.Report, error){finish(appReport)},
			want:      appReport,
			wantCalls: []call{{left: 10 * time.Minute, analyzers: app}},
		},
		{
			name:         "application scan runs out of time",
			analyzers:    app,
			timeout:      12 * time.Minute,
			scans:        []func(context.Context) (*types.Report, error){block, finish(osReport)},
			want:         osReport,
			wantFallback: true,
			wantCalls:    []call{{left: 10 * time.Minute, analyzers: app}, {left: 2 * time.Minute, analyzers: osOnly}},
		},
		{
			name:         "application scan runs out of time with another error",
			analyzers:    app,
			timeout:      12 * time.Minute,
			scans:        []func(context.Context) (*types.Report, error){blockFail, finish(osReport)},
			want:         osReport,
			wantFallback: true,
			wantCalls:    []call{{left: 10 * time.Minute, analyzers: app}, {left: 2 * time.Minute, analyzers: osOnly}},
		},
		{
			name:      "OS rescan fails",
			analyzers: app,
			timeout:   12 * time.Minute,
			scans:     []func(context.Context) (*types.Report, error){block, fail(errScan)},
			wantErr:   errScan,
			wantCalls: []call{{left: 10 * time.Minute, analyzers: app}, {left: 2 * time.Minute, analyzers: osOnly}},
		},
		{
			name:      "application scan fails",
			analyzers: app,
			timeout:   12 * time.Minute,
			scans:     []func(context.Context) (*types.Report, error){fail(errScan)},
			wantErr:   errScan,
			wantCalls: []call{{left: 10 * time.Minute, analyzers: app}},
		},
		{
			name:      "application scan overruns the wall",
			analyzers: app,
			timeout:   12 * time.Minute,
			scans:     []func(context.Context) (*types.Report, error){overrun},
			wantErr:   errScan,
			wantCalls: []call{{left: 10 * time.Minute, analyzers: app}},
		},
		{
			name:      "cancelled",
			analyzers: app,
			timeout:   12 * time.Minute,
			cancel:    true,
			scans:     []func(context.Context) (*types.Report, error){block},
			wantErr:   context.Canceled,
			wantCalls: []call{{left: 10 * time.Minute, analyzers: app}},
		},
		{
			name:         "just over the reserve left",
			analyzers:    app,
			timeout:      2*time.Minute + 10*time.Second,
			scans:        []func(context.Context) (*types.Report, error){block, finish(osReport)},
			want:         osReport,
			wantFallback: true,
			wantCalls:    []call{{left: 10 * time.Second, analyzers: app}, {left: 2 * time.Minute, analyzers: osOnly}},
		},
		{
			name:      "exactly the reserve left",
			analyzers: app,
			timeout:   2 * time.Minute,
			scans:     []func(context.Context) (*types.Report, error){finish(appReport)},
			want:      appReport,
			wantCalls: []call{{left: time.Minute, analyzers: app}},
		},
		{
			name:         "exactly the reserve left, application scan runs out of time",
			analyzers:    app,
			timeout:      2 * time.Minute,
			scans:        []func(context.Context) (*types.Report, error){block, finish(osReport)},
			want:         osReport,
			wantFallback: true,
			wantCalls:    []call{{left: time.Minute, analyzers: app}, {left: time.Minute, analyzers: osOnly}},
		},
		{
			name:      "less time left than the reserve",
			analyzers: app,
			timeout:   time.Minute,
			scans:     []func(context.Context) (*types.Report, error){finish(appReport)},
			want:      appReport,
			wantCalls: []call{{left: 30 * time.Second, analyzers: app}},
		},
		{
			name:         "less time left than the reserve, application scan runs out of time",
			analyzers:    app,
			timeout:      time.Minute,
			scans:        []func(context.Context) (*types.Report, error){block, finish(osReport)},
			want:         osReport,
			wantFallback: true,
			wantCalls:    []call{{left: 30 * time.Second, analyzers: app}, {left: 30 * time.Second, analyzers: osOnly}},
		},
		{
			name:         "budget of a flare",
			analyzers:    app,
			timeout:      10 * time.Second,
			scans:        []func(context.Context) (*types.Report, error){block, finish(osReport)},
			want:         osReport,
			wantFallback: true,
			wantCalls:    []call{{left: 5 * time.Second, analyzers: app}, {left: 5 * time.Second, analyzers: osOnly}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if tt.timeout > 0 {
					var cancelTimeout context.CancelFunc
					ctx, cancelTimeout = context.WithTimeout(ctx, tt.timeout)
					defer cancelTimeout()
				}

				var calls []call
				var options []artifact.Option
				report, osFallback, err := scanWithOSFallback(ctx, "target", sbom.ScanOptions{Analyzers: tt.analyzers}, func(ctx context.Context, option artifact.Option) (*types.Report, error) {
					var left time.Duration
					if deadline, ok := ctx.Deadline(); ok {
						left = time.Until(deadline)
					}
					calls = append(calls, call{left: left})
					options = append(options, option)
					if len(calls) > len(tt.scans) {
						t.Fatalf("scan called %d times, want at most %d", len(calls), len(tt.scans))
					}
					if tt.cancel {
						cancel()
					}
					return tt.scans[len(calls)-1](ctx)
				})

				assert.Equal(t, tt.want, report)
				assert.Equal(t, tt.wantFallback, osFallback, "OS fallback")
				if tt.wantErr != nil {
					assert.ErrorIs(t, err, tt.wantErr)
				} else {
					assert.NoError(t, err)
				}
				require.Len(t, calls, len(tt.wantCalls))
				for i, want := range tt.wantCalls {
					assert.Equal(t, want.left, calls[i].left, "call %d: time to the deadline", i)
					wantOption := getDefaultArtifactOption(sbom.ScanOptions{Analyzers: want.analyzers})
					assert.ElementsMatch(t, wantOption.DisabledAnalyzers, options[i].DisabledAnalyzers, "call %d: disabled analyzers", i)
					// The deadline alone bounds a scan that has one.
					if tt.timeout > 0 {
						wantOption.AnalyzerTimeout, wantOption.PostAnalyzerTimeout = 0, 0
					}
					assert.Equal(t, wantOption.AnalyzerTimeout, options[i].AnalyzerTimeout, "call %d: analyzer timeout", i)
					assert.Equal(t, wantOption.PostAnalyzerTimeout, options[i].PostAnalyzerTimeout, "call %d: post-analyzer timeout", i)
				}
			})
		})
	}
}

// TestScanFilesystemAnalyzers scans a root holding an apk package and an npm
// package. A scan with the application analyzers lists both, with or without
// a deadline, and a scan with the OS analyzers lists the apk package.
// OSFallback is false for every report.
func TestScanFilesystemAnalyzers(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{
		"etc/os-release":                         "ID=alpine\nVERSION_ID=3.20.0\n",
		"lib/apk/db/installed":                   "P:musl\nV:1.2.5-r0\nA:x86_64\no:musl\n\n",
		"app/node_modules/left-pad/package.json": `{"name": "left-pad", "version": "1.3.0"}`,
	} {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
	}

	app := []string{OSAnalyzers, LanguagesAnalyzers}
	tests := []struct {
		name      string
		analyzers []string
		timeout   time.Duration // 0 for no deadline
		want      []string      // purl types
	}{
		{
			name:      "application analyzers without a deadline",
			analyzers: app,
			want:      []string{"apk", "npm"},
		},
		{
			name:      "application analyzers with time to spare",
			analyzers: app,
			timeout:   10 * time.Minute,
			want:      []string{"apk", "npm"},
		},
		{
			name:      "os analyzers",
			analyzers: []string{OSAnalyzers},
			timeout:   10 * time.Minute,
			want:      []string{"apk"},
		},
	}
	c := NewCollectorForCLI()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}

			report, err := c.ScanFilesystem(ctx, root, sbom.ScanOptions{Analyzers: tt.analyzers}, true)
			require.NoError(t, err)
			assert.False(t, report.OSFallback(), "OS fallback")

			var got []string
			for _, component := range report.ToCycloneDX().GetComponents() {
				if purlType, _, ok := strings.Cut(strings.TrimPrefix(component.GetPurl(), "pkg:"), "/"); ok {
					got = append(got, purlType)
				}
			}
			slices.Sort(got)
			assert.Equal(t, tt.want, slices.Compact(got))
		})
	}
}

// TestReportHash checks that the ID of a fallback differs from the ID of a full
// scan listing the same results, for the sbom check to send the full scan in
// full after the fallback rather than as a heartbeat.
func TestReportHash(t *testing.T) {
	results := types.Results{{
		Target:   "root",
		Class:    types.ClassOSPkg,
		Packages: []ftypes.Package{{Name: "musl", Version: "1.2.5-r0"}},
	}}
	full, err := reportHash(results, false)
	require.NoError(t, err)
	again, err := reportHash(results, false)
	require.NoError(t, err)
	fallback, err := reportHash(results, true)
	require.NoError(t, err)

	assert.Equal(t, full, again)
	assert.NotEqual(t, full, fallback)
}

// TestReportOSFallback checks that a report built from a fallback tells so,
// and carries the metadata property for the readers of the SBOM.
func TestReportOSFallback(t *testing.T) {
	for _, osFallback := range []bool{false, true} {
		report, err := newReport("id", &types.Report{ArtifactName: "root", ArtifactType: ftypes.TypeFilesystem}, cyclonedx.NewMarshaler(""), reportOptions{osFallback: osFallback})
		require.NoError(t, err)
		assert.Equal(t, osFallback, report.OSFallback())

		var values []string
		for _, property := range report.ToCycloneDX().GetMetadata().GetProperties() {
			if property.GetName() == sbom.OSFallbackProperty {
				values = append(values, property.GetValue())
			}
		}
		if osFallback {
			assert.Equal(t, []string{"true"}, values)
		} else {
			assert.Empty(t, values)
		}
	}
}
