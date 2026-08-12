// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package software

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestParseCBSPackageKey(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		wantIdentity string
		wantVersion  string
		wantOK       bool
	}{
		{
			name:         "KB package",
			key:          "Package_for_KB5061234~31bf3856ad364e35~amd64~~10.0.1.5",
			wantIdentity: "KB5061234",
			wantVersion:  "10.0.1.5",
			wantOK:       true,
		},
		{
			name:         "six digit KB",
			key:          "Package_for_KB506123~31bf3856ad364e35~amd64~~10.0.1.0",
			wantIdentity: "KB506123",
			wantVersion:  "10.0.1.0",
			wantOK:       true,
		},
		{
			name:         "lowercase kb is normalized",
			key:          "package_for_kb5061234~31bf3856ad364e35~amd64~~10.0.1.5",
			wantIdentity: "KB5061234",
			wantVersion:  "10.0.1.5",
			wantOK:       true,
		},
		{
			// Monthly cumulative updates look like this on Windows 10 and 11: no KB
			// number anywhere in the key, so they must be identified by family.
			name:         "cumulative rollup without a KB number",
			key:          "Package_for_RollupFix~31bf3856ad364e35~amd64~~19041.2075.1.1",
			wantIdentity: "RollupFix",
			wantVersion:  "19041.2075.1.1",
			wantOK:       true,
		},
		{
			name:         "servicing stack update",
			key:          "Package_for_ServicingStack_2075~31bf3856ad364e35~amd64~~19041.2075.1.0",
			wantIdentity: "ServicingStack_2075",
			wantVersion:  "19041.2075.1.0",
			wantOK:       true,
		},
		{
			name:   "component manifest",
			key:    "Microsoft-Windows-Foo-Package~31bf3856ad364e35~amd64~~10.0.1.0",
			wantOK: false,
		},
		{
			name:         "no version component",
			key:          "Package_for_KB5061234~",
			wantIdentity: "KB5061234",
			wantVersion:  "",
			wantOK:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity, version, ok := parseCBSPackageKey(tt.key)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantIdentity, identity)
				assert.Equal(t, tt.wantVersion, version)
			}
		})
	}
}

func TestCBSInstallTime(t *testing.T) {
	t.Run("converts a FILETIME to an RFC3339 UTC timestamp", func(t *testing.T) {
		want := time.Date(2025, 6, 10, 12, 30, 15, 0, time.UTC)
		ft := windows.NsecToFiletime(want.UnixNano())

		installDate, installTime := cbsInstallTime(ft.HighDateTime, ft.LowDateTime)

		assert.Equal(t, want.Format(time.RFC3339Nano), installDate)
		assert.Equal(t, want.UnixNano(), installTime)
	})

	t.Run("zero FILETIME yields no install date", func(t *testing.T) {
		installDate, installTime := cbsInstallTime(0, 0)

		assert.Empty(t, installDate)
		assert.Zero(t, installTime)
	})
}

func TestBuildOSUpdateEntries(t *testing.T) {
	t.Run("dedupes packages servicing the same KB", func(t *testing.T) {
		older := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		newer := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

		entries := buildOSUpdateEntries([]cbsPackage{
			{identity: "KB5061234", version: "10.0.1.0", installDate: older.Format(time.RFC3339Nano), installTime: older.UnixNano()},
			{identity: "KB5061234", version: "10.0.2.0", installDate: newer.Format(time.RFC3339Nano), installTime: newer.UnixNano()},
		})

		require.Len(t, entries, 1)
		assert.Equal(t, "10.0.2.0", entries[0].Version)
		assert.Equal(t, newer.Format(time.RFC3339Nano), entries[0].InstallDate)
	})

	t.Run("breaks install-time ties on version", func(t *testing.T) {
		stamp := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

		entries := buildOSUpdateEntries([]cbsPackage{
			{identity: "KB5061234", version: "10.0.9.0", installTime: stamp.UnixNano()},
			{identity: "KB5061234", version: "10.0.10.0", installTime: stamp.UnixNano()},
		})

		require.Len(t, entries, 1)
		assert.Equal(t, "10.0.10.0", entries[0].Version)
	})

	t.Run("builds one KB-identified entry per update", func(t *testing.T) {
		entries := buildOSUpdateEntries([]cbsPackage{
			{identity: "KB5061234", version: "10.0.1.5", installDate: "2025-06-10T12:30:15Z"},
			{identity: "KB5000001", version: "10.0.1.0"},
		})

		require.Len(t, entries, 2)
		// Entries are sorted by product code for a stable snapshot.
		assert.Equal(t, "KB5000001", entries[0].ProductCode)

		kb := entries[1]
		assert.Equal(t, "KB5061234", kb.ProductCode)
		assert.Equal(t, "Update KB5061234", kb.DisplayName)
		assert.Equal(t, "10.0.1.5", kb.Version)
		assert.Equal(t, "2025-06-10T12:30:15Z", kb.InstallDate)
		assert.Equal(t, softwareTypeOSUpdate, kb.Source)
		assert.Equal(t, "Microsoft Corporation", kb.Publisher)
		assert.Equal(t, "installed", kb.Status)
	})

	t.Run("rolling families collapse to one versioned entry", func(t *testing.T) {
		// Successive monthly cumulative updates keep the RollupFix identity, so they
		// must read as a version change on a single entry rather than remove + install.
		lastMonth := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
		thisMonth := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

		entries := buildOSUpdateEntries([]cbsPackage{
			{identity: "RollupFix", version: "19041.1949.1.6", installDate: lastMonth.Format(time.RFC3339Nano), installTime: lastMonth.UnixNano()},
			{identity: "RollupFix", version: "19041.2075.1.1", installDate: thisMonth.Format(time.RFC3339Nano), installTime: thisMonth.UnixNano()},
		})

		require.Len(t, entries, 1)
		assert.Equal(t, "RollupFix", entries[0].ProductCode)
		assert.Equal(t, "Update RollupFix", entries[0].DisplayName)
		assert.Equal(t, "19041.2075.1.1", entries[0].Version)
	})

	t.Run("no packages yields no entries", func(t *testing.T) {
		assert.Empty(t, buildOSUpdateEntries(nil))
	})
}
