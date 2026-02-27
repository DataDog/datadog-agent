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
