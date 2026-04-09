// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// generateIR is a test helper that generates IR for the given probe names.
func generateIR(t *testing.T, probeNames ...string) *ir.Program {
	t.Helper()
	cfgs := testprogs.MustGetCommonConfigs(t)
	cfg := cfgs[0]
	bin := testprogs.MustGetBinary(t, "sample", cfg)
	probes := testprogs.MustGetProbeDefinitions(t, "sample")
	probes = slices.DeleteFunc(probes, func(p ir.ProbeDefinition) bool {
		return !slices.Contains(probeNames, p.GetID())
	})
	require.Len(t, probes, len(probeNames))
	obj, err := object.OpenElfFileWithDwarf(bin)
	require.NoError(t, err)
	defer obj.Close()
	prog, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	return prog
}

// findReturnEvent finds the return event for a probe's first instance.
func findReturnEvent(t *testing.T, probe *ir.Probe) *ir.Event {
	t.Helper()
	require.NotEmpty(t, probe.Instances, "probe %s has no instances", probe.GetID())
	for _, ev := range probe.Instances[0].Events {
		if ev.Kind == ir.EventKindReturn {
			return ev
		}
	}
	t.Fatalf("no return event found for probe %s", probe.GetID())
	return nil
}

// returnExprNames returns the names of all Local expressions in a return event.
func returnExprNames(ev *ir.Event) []string {
	var names []string
	for _, expr := range ev.Type.Expressions {
		if expr.Kind == ir.RootExpressionKindLocal {
			names = append(names, expr.Name)
		}
	}
	return names
}

// TestReturnExprSingleUnnamed verifies that a function with a single unnamed
// return value produces an expression named "@return" (not "~r0").
func TestReturnExprSingleUnnamed(t *testing.T) {
	prog := generateIR(t, "testReturnsInt")
	probe := prog.Probes[0]
	ev := findReturnEvent(t, probe)

	names := returnExprNames(ev)
	require.Equal(t, []string{"@return"}, names,
		"single unnamed return should produce @return, not DWARF name")
}

// TestReturnExprSingleNamed verifies that a function with a single named
// return value produces an expression named "@return" (not the user name).
func TestReturnExprSingleNamed(t *testing.T) {
	prog := generateIR(t, "testNamedReturn")
	probe := prog.Probes[0]
	ev := findReturnEvent(t, probe)

	names := returnExprNames(ev)
	require.Equal(t, []string{"@return"}, names,
		"single named return should produce @return, not 'result'")
}

// TestReturnExprMultipleUnnamed verifies that a function with multiple unnamed
// returns produces expressions with tilde-stripped names (r0, r1, ...).
func TestReturnExprMultipleUnnamed(t *testing.T) {
	prog := generateIR(t, "testReturnsPrimitives")
	probe := prog.Probes[0]
	ev := findReturnEvent(t, probe)

	names := returnExprNames(ev)
	// testReturnsPrimitives returns (int8, int16, int32, int64, uint8, uint16, uint32, uint64)
	// DWARF names: ~r0 through ~r7; after stripping: r0 through r7
	for i, name := range names {
		require.Falsef(t, strings.HasPrefix(name, "~"),
			"return expression %d should not have ~ prefix, got %q", i, name)
	}
	require.Len(t, names, 8)
	require.Equal(t, "r0", names[0])
	require.Equal(t, "r7", names[7])
}

// TestReturnExprMultipleNamed verifies that named return values preserve
// their user-chosen names (without tilde prefix since they don't have one).
func TestReturnExprMultipleNamed(t *testing.T) {
	prog := generateIR(t, "testMultipleNamedReturn")
	probe := prog.Probes[0]
	ev := findReturnEvent(t, probe)

	names := returnExprNames(ev)
	require.Equal(t, []string{"result", "result2"}, names,
		"named returns should preserve user-chosen names")
}

// TestReturnExprMixed verifies functions with a mix of named and unnamed
// returns strip tildes from unnamed ones while preserving named ones.
func TestReturnExprMixed(t *testing.T) {
	prog := generateIR(t, "testSomeNamedReturn")
	probe := prog.Probes[0]
	ev := findReturnEvent(t, probe)

	names := returnExprNames(ev)
	// testSomeNamedReturn returns (~r0, result2, ~r2) -> (r0, result2, r2)
	require.Equal(t, []string{"r0", "result2", "r2"}, names,
		"mixed returns: unnamed should be stripped, named preserved")
}
