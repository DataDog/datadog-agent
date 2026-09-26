// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	cdx "github.com/CycloneDX/cyclonedx-go"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/sbom"
)

func TestAppendSBOMImageCreated(t *testing.T) {
	config := func(created time.Time) *v1.ConfigFile {
		return &v1.ConfigFile{Created: v1.Time{Time: created}}
	}
	bom := func(props ...cdx.Property) *cdx.BOM {
		b := &cdx.BOM{Metadata: &cdx.Metadata{Component: &cdx.Component{}}}
		if props != nil {
			b.Metadata.Component.Properties = &props
		}
		return b
	}

	tests := []struct {
		name   string
		bom    *cdx.BOM
		config *v1.ConfigFile
		want   string
	}{
		{
			name:   "offset is normalized to UTC",
			bom:    bom(),
			config: config(time.Date(2024, time.November, 7, 5, 26, 15, 0, time.FixedZone("UTC+5", 5*60*60))),
			want:   "2024-11-07T00:26:15Z",
		},
		{
			name:   "nanoseconds are preserved",
			bom:    bom(),
			config: config(time.Date(2024, time.November, 7, 0, 26, 15, 123456789, time.UTC)),
			want:   "2024-11-07T00:26:15.123456789Z",
		},
		{
			name:   "reproducible build has no created time",
			bom:    bom(),
			config: config(time.Time{}),
		},
		{
			name: "missing image config",
			bom:  bom(),
		},
		{
			name:   "existing property wins",
			bom:    bom(cdx.Property{Name: imageCreatedPropertyKey, Value: "2020-01-02T03:04:05Z"}),
			config: config(time.Date(2024, time.November, 7, 0, 26, 15, 0, time.UTC)),
			want:   "2020-01-02T03:04:05Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appendSBOMImageCreated(tt.bom, tt.config)

			var got []string
			if props := tt.bom.Metadata.Component.Properties; props != nil {
				for _, p := range *props {
					if p.Name == imageCreatedPropertyKey {
						got = append(got, p.Value)
					}
				}
			}
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			assert.Equal(t, tt.want, got[0])
		})
	}
}

// TestScanFilesystemCarriesImageCreated covers the use_mount path, where the
// caller supplies the build time for a local artifact.
func TestScanFilesystemCarriesImageCreated(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc", "os-release"), []byte("ID=alpine\nVERSION_ID=3.20.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	created := time.Date(2024, time.November, 7, 0, 26, 15, 0, time.UTC)
	c := NewCollectorForCLI()

	report, err := c.scanFilesystem(context.Background(), root, sbom.ScanOptions{Analyzers: []string{OSAnalyzers}}, false, created)
	if err != nil {
		t.Fatalf("scanFilesystem: %v", err)
	}
	var got []string
	for _, p := range report.ToCycloneDX().GetMetadata().GetComponent().GetProperties() {
		if p.GetName() == imageCreatedPropertyKey {
			got = append(got, p.GetValue())
		}
	}
	assert.Equal(t, []string{"2024-11-07T00:26:15Z"}, got)

	hostReport, err := c.ScanFilesystem(context.Background(), root, sbom.ScanOptions{Analyzers: []string{OSAnalyzers}}, false)
	if err != nil {
		t.Fatalf("ScanFilesystem: %v", err)
	}
	for _, p := range hostReport.ToCycloneDX().GetMetadata().GetComponent().GetProperties() {
		assert.NotEqual(t, imageCreatedPropertyKey, p.GetName(), "host scan reports an image build time")
	}
}
