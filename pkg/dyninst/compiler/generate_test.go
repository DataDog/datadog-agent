// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

package compiler

import (
	"bufio"
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TestCorruptedLocationRecovery tests that GenerateProgram does not fail
// when a location list entry has an invalid register piece size. Instead
// of failing the entire compilation, the expression should be treated
// as unavailable and a log message should be emitted.
func TestCorruptedLocationRecovery(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			testCorruptedLocationRecovery(t, cfg)
		})
	}
}

func testCorruptedLocationRecovery(t *testing.T, cfg testprogs.Config) {
	// Set up a logger that captures output so we can assert on log messages.
	var logBuf bytes.Buffer
	w := bufio.NewWriter(&logBuf)
	l, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, log.DebugLvl)
	require.NoError(t, err)
	log.SetupLogger(l, "debug")
	t.Cleanup(func() {
		log.SetupLogger(log.Default(), "debug")
	})

	binPath := testprogs.MustGetBinary(t, "simple", cfg)
	probeDefs := testprogs.MustGetProbeDefinitions(t, "simple")
	probeDefs = slices.DeleteFunc(probeDefs, testprogs.HasIssueTag)
	obj, err := object.OpenElfFileWithDwarf(binPath)
	require.NoError(t, err)
	defer func() { _ = obj.Close() }()
	program, err := irgen.GenerateIR(1, obj, probeDefs)
	require.NoError(t, err)
	require.Empty(t, program.Issues)

	// Find a Register piece and corrupt its size to be > 8 bytes,
	// which is an invalid state for a CPU register.
	corrupted := corruptRegisterPieceSize(t, program)
	require.True(t, corrupted, "failed to find a Register piece to corrupt in the IR program")

	// GenerateProgram should succeed despite the corrupted location.
	_, err = GenerateProgram(program)
	require.NoError(t, err)

	w.Flush()
	logOutput := logBuf.String()
	assert.True(t,
		strings.Contains(logOutput, "unsupported register size: 16"),
		"expected log message about unsupported register size, got: %s", logOutput,
	)
}

// TestMultiFieldStructExpressionEncoding tests that when a template like
// "{a.a} {a.b} {a.c}" references multiple fields of a struct, the compiler
// emits read ops for every field — not just the first one. This is the
// regression test for DEBUG-5245 where EncodeLocationOp only processed the
// first layout piece when a single CFA loclist piece covered multiple fields.
func TestMultiFieldStructExpressionEncoding(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			testMultiFieldStructExpressionEncoding(t, cfg)
		})
	}
}

func testMultiFieldStructExpressionEncoding(t *testing.T, cfg testprogs.Config) {
	binPath := testprogs.MustGetBinary(t, "sample", cfg)
	probeDefs := testprogs.MustGetProbeDefinitions(t, "sample")
	probeDefs = slices.DeleteFunc(probeDefs, testprogs.HasIssueTag)

	// Keep only the testThreeStringsInStructExpr probe.
	probeDefs = slices.DeleteFunc(probeDefs, func(p ir.ProbeDefinition) bool {
		return p.GetID() != "testThreeStringsInStructExpr"
	})
	require.Len(t, probeDefs, 1, "expected exactly one probe definition for testThreeStringsInStructExpr")

	obj, err := object.OpenElfFileWithDwarf(binPath)
	require.NoError(t, err)
	defer func() { _ = obj.Close() }()

	irProg, err := irgen.GenerateIR(1, obj, probeDefs)
	require.NoError(t, err)

	compiled, err := GenerateProgram(irProg)
	require.NoError(t, err)

	// Collect ProcessExpression functions for this probe (one per expression: a.a, a.b, a.c).
	var exprFuncs []Function
	for _, fn := range compiled.Functions {
		if _, ok := fn.ID.(ProcessExpression); ok {
			exprFuncs = append(exprFuncs, fn)
		}
	}
	require.Len(t, exprFuncs, 3, "expected 3 ProcessExpression functions for {a.a} {a.b} {a.c}")

	// For each expression function, collect the read ops (CFA dereference or register read)
	// that occur between ExprPrepareOp and ExprSaveOp.
	outputOffsets := make([]uint32, 0, 3)
	for i, fn := range exprFuncs {
		var readOps []Op
		for _, op := range fn.Ops {
			switch op.(type) {
			case ExprDereferenceCfaOp, ExprReadRegisterOp:
				readOps = append(readOps, op)
			}
		}
		assert.NotEmpty(t, readOps,
			"expression %d has no read ops — field would be silently dropped (DEBUG-5245 regression)", i)

		// Record the OutputOffset of the first read op for distinctness check.
		if len(readOps) > 0 {
			switch op := readOps[0].(type) {
			case ExprDereferenceCfaOp:
				outputOffsets = append(outputOffsets, op.OutputOffset)
			case ExprReadRegisterOp:
				outputOffsets = append(outputOffsets, op.OutputOffset)
			}
		}
	}

	// The three expressions should write to distinct output offsets since
	// they capture different struct fields at different positions.
	require.Len(t, outputOffsets, 3, "expected 3 output offsets")
	assert.NotEqual(t, outputOffsets[0], outputOffsets[1],
		"expressions 0 and 1 should have different OutputOffset values")
	assert.NotEqual(t, outputOffsets[1], outputOffsets[2],
		"expressions 1 and 2 should have different OutputOffset values")
	assert.NotEqual(t, outputOffsets[0], outputOffsets[2],
		"expressions 0 and 2 should have different OutputOffset values")
}

// corruptRegisterPieceSize walks the IR program to find a Register location
// piece and sets its Size to an invalid value (> 8). Returns true if a piece
// was corrupted.
func corruptRegisterPieceSize(t *testing.T, program *ir.Program) bool {
	t.Helper()
	for _, probe := range program.Probes {
		for _, event := range probe.Events {
			for _, expr := range event.Type.Expressions {
				for _, op := range expr.Expression.Operations {
					locOp, ok := op.(*ir.LocationOp)
					if !ok {
						continue
					}
					for i := range locOp.Variable.Locations {
						loc := &locOp.Variable.Locations[i]
						for j := range loc.Pieces {
							if _, ok := loc.Pieces[j].Op.(ir.Register); ok {
								loc.Pieces[j].Size = 16
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}
