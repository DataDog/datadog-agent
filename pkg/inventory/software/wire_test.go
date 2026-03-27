// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package software

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEntryToWire_WireToEntry_RoundTrip verifies that converting an Entry to
// SoftwareInventoryWireEntry and back preserves all fields.
func TestEntryToWire_WireToEntry_RoundTrip(t *testing.T) {
	entry := &Entry{
		Source:        "app",
		DisplayName:   "TestApp",
		Version:       "1.2.3",
		InstallDate:   "2024-01-15T10:00:00Z",
		UserSID:       "S-1-5-21-xxx",
		Is64Bit:       true,
		Publisher:     "Test Corp",
		Status:        "installed",
		BrokenReason:  "executable not found",
		ProductCode:   "com.example.app",
		InstallSource: "pkg",
		PkgID:         "com.example.pkg",
		InstallPath:   "/Applications/TestApp.app",
		InstallPaths:  []string{"/usr/local/bin", "/usr/local/lib"},
	}
	wire := EntryToWire(entry)
	got := WireToEntry(&wire)
	assert.Equal(t, entry.Source, got.Source)
	assert.Equal(t, entry.DisplayName, got.DisplayName)
	assert.Equal(t, entry.Version, got.Version)
	assert.Equal(t, entry.InstallDate, got.InstallDate)
	assert.Equal(t, entry.UserSID, got.UserSID)
	assert.Equal(t, entry.Is64Bit, got.Is64Bit)
	assert.Equal(t, entry.Publisher, got.Publisher)
	assert.Equal(t, entry.Status, got.Status)
	assert.Equal(t, entry.BrokenReason, got.BrokenReason)
	assert.Equal(t, entry.ProductCode, got.ProductCode)
	assert.Equal(t, entry.InstallSource, got.InstallSource)
	assert.Equal(t, entry.PkgID, got.PkgID)
	assert.Equal(t, entry.InstallPath, got.InstallPath)
	assert.Equal(t, entry.InstallPaths, got.InstallPaths)
}

// TestEntryToWire_NilEntry verifies that EntryToWire(nil) returns a zero-valued
// SoftwareInventoryWireEntry without panicking.
func TestEntryToWire_NilEntry(t *testing.T) {
	wire := EntryToWire(nil)
	assert.Empty(t, wire.Source)
	assert.Empty(t, wire.DisplayName)
	assert.Empty(t, wire.InstallPath)
	assert.Nil(t, wire.InstallPaths)
}

// TestWireToEntry_NilWire verifies that WireToEntry(nil) returns a zero-valued
// Entry without panicking.
func TestWireToEntry_NilWire(t *testing.T) {
	got := WireToEntry(nil)
	assert.Empty(t, got.Source)
	assert.Empty(t, got.DisplayName)
	assert.Empty(t, got.InstallPath)
	assert.Nil(t, got.InstallPaths)
}
