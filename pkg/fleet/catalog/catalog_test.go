// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package catalog

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	want := Catalog{Packages: []Package{{
		Name:     "package",
		Version:  "1.2.3",
		SHA256:   "digest",
		URL:      "https://example.com/package",
		Size:     42,
		Platform: "linux",
		Arch:     "arm64",
	}}}

	got, err := Parse([]byte(`{"packages":[{"package":"package","version":"1.2.3","sha256":"digest","url":"https://example.com/package","size":42,"platform":"linux","arch":"arm64"}]}`))
	require.NoError(t, err)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	if _, err := Parse([]byte(`{"packages":`)); err == nil {
		t.Fatal("Parse() error = nil, want an error")
	}
}

func TestMergePreservesCatalogAndPackageOrder(t *testing.T) {
	first := Package{Name: "first"}
	second := Package{Name: "second"}
	third := Package{Name: "third"}

	got := Merge(
		Catalog{Packages: []Package{first, second}},
		Catalog{Packages: []Package{third}},
	)
	want := Catalog{Packages: []Package{first, second, third}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Merge() = %#v, want %#v", got, want)
	}
}

func TestPackageMatchesTarget(t *testing.T) {
	tests := []struct {
		name     string
		pkg      Package
		platform string
		arch     string
		want     bool
	}{
		{
			name:     "portable package",
			pkg:      Package{},
			platform: "linux",
			arch:     "arm64",
			want:     true,
		},
		{
			name:     "platform-only constraint",
			pkg:      Package{Platform: "linux"},
			platform: "linux",
			arch:     "arm64",
			want:     true,
		},
		{
			name:     "architecture-only constraint",
			pkg:      Package{Arch: "arm64"},
			platform: "darwin",
			arch:     "arm64",
			want:     true,
		},
		{
			name:     "platform mismatch",
			pkg:      Package{Platform: "linux"},
			platform: "darwin",
			arch:     "arm64",
		},
		{
			name:     "architecture mismatch",
			pkg:      Package{Arch: "amd64"},
			platform: "linux",
			arch:     "arm64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pkg.MatchesTarget(tt.platform, tt.arch); got != tt.want {
				t.Fatalf("MatchesTarget() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestGetPackage(t *testing.T) {
	portable := Package{Name: "package", Version: "1.0.0", URL: "https://example.com/portable"}
	linuxAMD64 := Package{Name: "package", Version: "1.0.0", URL: "https://example.com/linux-amd64", Platform: "linux", Arch: "amd64"}
	duplicate := Package{Name: "package", Version: "1.0.0", URL: "https://example.com/duplicate"}
	catalog := Catalog{Packages: []Package{
		{Name: "other", Version: "1.0.0", URL: "https://example.com/other"},
		{Name: "package", Version: "2.0.0", URL: "https://example.com/other-version"},
		linuxAMD64,
		portable,
		duplicate,
	}}

	tests := []struct {
		name     string
		platform string
		arch     string
		want     Package
		wantOK   bool
	}{
		{
			name:     "target-specific package",
			platform: "linux",
			arch:     "amd64",
			want:     linuxAMD64,
			wantOK:   true,
		},
		{
			name:     "portable package",
			platform: "darwin",
			arch:     "arm64",
			want:     portable,
			wantOK:   true,
		},
		{
			name:     "first matching package",
			platform: "windows",
			arch:     "amd64",
			want:     portable,
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := catalog.GetPackage("package", "1.0.0", tt.platform, tt.arch)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("GetPackage() = (%#v, %t), want (%#v, %t)", got, ok, tt.want, tt.wantOK)
			}
		})
	}

	if got, ok := catalog.GetPackage("missing", "1.0.0", "linux", "amd64"); ok {
		t.Fatalf("GetPackage() = (%#v, true), want no match", got)
	}
}

func TestPackageValidate(t *testing.T) {
	validDigestURL := "oci://example.com/package@sha256:2a5ca68f1f0a088cdf1cd1efa086ffe0ca80f8339c7fa12a7f41bbe9d1527cb6"
	tests := []struct {
		name    string
		pkg     Package
		wantErr bool
	}{
		{
			name: "HTTPS URL",
			pkg:  Package{Name: "package", Version: "1.0.0", URL: "https://example.com/package"},
		},
		{
			name: "OCI digest URL",
			pkg:  Package{Name: "package", Version: "1.0.0", URL: validDigestURL},
		},
		{
			name:    "empty name",
			pkg:     Package{Version: "1.0.0", URL: "https://example.com/package"},
			wantErr: true,
		},
		{
			name:    "empty version",
			pkg:     Package{Name: "package", URL: "https://example.com/package"},
			wantErr: true,
		},
		{
			name:    "empty URL",
			pkg:     Package{Name: "package", Version: "1.0.0"},
			wantErr: true,
		},
		{
			name:    "malformed URL",
			pkg:     Package{Name: "package", Version: "1.0.0", URL: "https://example.com/%"},
			wantErr: true,
		},
		{
			name:    "mutable OCI tag",
			pkg:     Package{Name: "package", Version: "1.0.0", URL: "oci://example.com/package:1.0.0"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pkg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestCatalogValidate(t *testing.T) {
	catalog := Catalog{Packages: []Package{
		{Name: "valid", Version: "1.0.0", URL: "https://example.com/valid"},
		{Name: "invalid", Version: "1.0.0"},
	}}
	if err := catalog.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want an error")
	}
}
