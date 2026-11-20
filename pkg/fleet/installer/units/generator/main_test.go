// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/darwinunits"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/linuxunits"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/units/windowsunits"
)

// TestGenerationIsUpToDate tests that the generated units are up to date.
//
// Regenerate them with:
// bazelisk run //pkg/fleet/installer/units/generator -- "$PWD/pkg/fleet/installer/units"
func TestGenerationIsUpToDate(t *testing.T) {
	if os.Getenv("CI") == "true" && runtime.GOOS == "darwin" {
		t.Skip("TestGenerationIsUpToDate is known to fail on the macOS Gitlab runners.")
	}
	generated := t.TempDir()
	require.NoError(t, generate(generated))
	for _, registry := range []string{"linuxunits", "darwinunits", "windowsunits"} {
		fixtures.AssertEqualFS(t, os.DirFS("../"+registry+"/gen"), os.DirFS(generated+"/"+registry+"/gen"))
	}
}

func TestRegistryEntriesAreUnique(t *testing.T) {
	type key struct {
		manager       units.Manager
		name, variant string
	}
	for _, all := range [][]units.Unit{linuxunits.All(), darwinunits.All(), windowsunits.All()} {
		seen := map[key]bool{}
		for _, u := range all {
			k := key{u.Manager, u.Name, u.Variant}
			assert.False(t, seen[k], "duplicate registry entry %+v", k)
			seen[k] = true
		}
	}
}

func TestEveryTemplateIsUsed(t *testing.T) {
	entries, err := templates.ReadDir("tmpl")
	require.NoError(t, err)
	used := map[string]bool{}
	for _, all := range [][]units.Unit{linuxunits.All(), darwinunits.All(), windowsunits.All()} {
		for _, u := range all {
			used[u.Template] = true
		}
	}
	for _, entry := range entries {
		assert.True(t, used[entry.Name()], "template %s is not used by any registry entry", entry.Name())
	}
}
