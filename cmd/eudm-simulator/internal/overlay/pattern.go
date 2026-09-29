// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package overlay applies scenario declarations to owned copies of captured telemetry.
package overlay

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

// Context identifies one deterministic emission. BaselineProcesses contains all
// chunks of the selected captured process cycle, never previously overlaid data.
type Context struct {
	Scenario             *schema.Scenario
	Group                schema.GroupDef
	Seed                 uint64
	DeviceOrdinal        int
	PhaseIndex           int
	Elapsed              time.Duration
	Stream               schema.Stream
	SampleOrdinal        int64
	ProcessSampleOrdinal int64
	BaselineProcesses    []*model.CollectorProc
}

// PatternValue evaluates a declared pattern with bounded, independently keyed
// variation. It does not select cohort membership or consume shared random state.
func PatternValue(ctx Context, pattern schema.Pattern, field string) (float64, error) {
	if ctx.Scenario == nil || ctx.PhaseIndex < 0 || ctx.PhaseIndex >= len(ctx.Scenario.Phases) {
		return 0, errors.New("overlay requires a valid phase")
	}
	phase := ctx.Scenario.Phases[ctx.PhaseIndex]
	if phase.Duration.Duration <= 0 {
		return 0, errors.New("overlay phase duration must be positive")
	}
	progress := min(1.0, max(0.0, float64(ctx.Elapsed)/float64(phase.Duration.Duration)))
	var value float64
	count := 0
	if p := pattern.Steady; p != nil {
		value = p.Value
		count++
	}
	if p := pattern.Ramp; p != nil {
		value = p.From + (p.To-p.From)*progress
		count++
	}
	if p := pattern.Step; p != nil {
		value = p.Before
		if progress >= p.At/100 {
			value = p.After
		}
		count++
	}
	if p := pattern.Spike; p != nil {
		sigma := p.Duration / 250
		if sigma <= 0 {
			return 0, errors.New("spike duration must be positive")
		}
		value = p.Baseline + (p.Peak-p.Baseline)*math.Exp(-math.Pow(progress-p.At/100, 2)/(2*sigma*sigma))
		count++
	}
	if count != 1 {
		return 0, errors.New("overlay requires exactly one pattern")
	}
	spread := ctx.Group.BaselineVariance
	if math.IsNaN(spread) || math.IsInf(spread, 0) || spread < 0 || spread > 1 {
		return 0, errors.New("overlay variation is outside [0,1]")
	}
	if spread != 0 {
		key := fmt.Sprintf("%d/%q/%d/%d/%q/%d/%q", ctx.Seed, ctx.Group.Group, ctx.DeviceOrdinal, ctx.PhaseIndex, ctx.Stream, ctx.SampleOrdinal, field)
		sum := sha256.Sum256([]byte(key))
		uniform := float64(binary.BigEndian.Uint64(sum[:8])>>11) / float64(uint64(1)<<53)
		// Both declared controls bound the spread; it never becomes larger than
		// the underlying value and cannot change cohort membership.
		spread = min(1.0, spread*phase.EffectiveJitterScale())
		value *= 1 + (2*uniform-1)*spread
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("overlay produced a nonfinite value")
	}
	return value, nil
}
