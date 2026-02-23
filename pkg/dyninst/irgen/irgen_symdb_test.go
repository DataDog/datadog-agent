// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package irgen_test

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// TestIRGenAndCompileSymDBProbes generates a symbol database for each test
// binary, creates a probe on every method and every injectable line reported by
// symdb, then runs both irgen.GenerateIR and compiler.GenerateProgram on the
// result. This is a stricter version of TestIRGenAllProbes: it derives probe
// targets from symdb (rather than raw ELF symbols) and additionally verifies
// that the compiler stage succeeds.
func TestIRGenAndCompileSymDBProbes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	programs := testprogs.MustGetPrograms(t)
	cfgs := testprogs.MustGetCommonConfigs(t)

	for _, pkg := range programs {
		switch pkg {
		case "simple", "sample":
		default:
			continue
		}
		// Choose a random configuration to test because each one is pretty slow
		// and there are only 4 or so supported, we'll get coverage of all
		// pretty quickly.
		cfgIdx := rand.Intn(len(cfgs))
		cfg := cfgs[cfgIdx]
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()
			t.Run(cfg.String(), func(t *testing.T) {
				t.Parallel()
				binPath := testprogs.MustGetBinary(t, pkg, cfg)
				testSymDBProbes(t, binPath)
			})
		})
	}
}

func testSymDBProbes(t *testing.T, binPath string) {
	// Extract symbols using symdb.
	symbols, err := symdb.ExtractSymbols(
		binPath,
		object.NewInMemoryLoader(),
		symdb.ExtractOptions{
			Scope: symdb.ExtractScopeAllSymbols,
		},
	)
	require.NoError(t, err)

	// Build probes from symdb data: one method probe per function/method, plus
	// one line probe per injectable line.
	var probes []ir.ProbeDefinition
	var methodCount, lineCount int
	probeID := 0

	for _, pkg := range symbols.Packages {
		probes, probeID, methodCount, lineCount = collectFunctionProbes(
			probes, probeID, methodCount, lineCount, pkg.Functions,
		)
		for _, typ := range pkg.Types {
			probes, probeID, methodCount, lineCount = collectFunctionProbes(
				probes, probeID, methodCount, lineCount, typ.Methods,
			)
		}
	}

	t.Logf("generated %d probes (%d method, %d line) from symdb for %s",
		len(probes), methodCount, lineCount, binPath)
	require.NotEmpty(t, probes)

	// Open the binary for IR generation.
	obj, err := object.OpenElfFileWithDwarf(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()

	// Generate IR.
	program, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.NotNil(t, program)

	// Verify the IR (same check as the existing test).
	verifyIR(t, program)

	// Compile the IR program.
	compiled, err := compiler.GenerateProgram(program)
	require.NoError(t, err)
	require.NotZero(t, compiled.ID)
}

// collectFunctionProbes appends a method probe for each function and a line
// probe for every injectable line within that function.
func collectFunctionProbes(
	probes []ir.ProbeDefinition,
	nextID, methodCount, lineCount int,
	functions []symdb.Function,
) ([]ir.ProbeDefinition, int, int, int) {
	for _, fn := range functions {
		// Method probe (probe on the function entry).
		probes = append(probes, &rcjson.SnapshotProbe{
			LogProbeCommon: rcjson.LogProbeCommon{
				ProbeCommon: rcjson.ProbeCommon{
					ID:    fmt.Sprintf("method_%d", nextID),
					Where: &rcjson.Where{MethodName: fn.QualifiedName},
				},
			},
		})
		nextID++
		methodCount++

		// Line probes: one per injectable line.
		for _, lr := range fn.InjectibleLines {
			for line := lr[0]; line <= lr[1]; line++ {
				probes = append(probes, &rcjson.SnapshotProbe{
					LogProbeCommon: rcjson.LogProbeCommon{
						ProbeCommon: rcjson.ProbeCommon{
							ID: fmt.Sprintf("line_%d", nextID),
							Where: &rcjson.Where{
								MethodName: fn.QualifiedName,
								SourceFile: fn.File,
								Lines:      []string{strconv.Itoa(line)},
							},
						},
					},
				})
				nextID++
				lineCount++
			}
		}
	}
	return probes, nextID, methodCount, lineCount
}
