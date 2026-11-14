// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
)

// PhaseBoundary is the start or the end of a measurement phase.
type PhaseBoundary string

const (
	// PhaseStart is the start of a phase, at the start of its window.
	PhaseStart PhaseBoundary = "start"
	// PhaseEnd is the end of a phase, at the end of its window.
	PhaseEnd PhaseBoundary = "end"
)

// PhaseEvent describes a boundary of a measurement phase, as an event that the dashboard of
// the benchmark overlays on its graphs, through its tags (see Tags), to show when each phase
// measured the Agents and where each version ran.
type PhaseEvent struct {
	Boundary PhaseBoundary
	// Phase is the index of the phase, from 0, among Phases.
	Phase  int
	Phases int
	Window Window
	// Baseline and Comparison are the images of the two versions.
	Baseline   []string
	Comparison []string
}

// installations returns the installations running the baseline and the comparison images in
// the phase.
func (e PhaseEvent) installations() (baselineOn, comparisonOn string) {
	return SwapInPhase(e.Phase, "a", "b")
}

// Time returns the time of the boundary.
func (e PhaseEvent) Time() time.Time {
	if e.Boundary == PhaseEnd {
		return e.Window.To
	}
	return e.Window.From
}

// Title returns the title of the event.
func (e PhaseEvent) Title() string {
	verb := "started"
	if e.Boundary == PhaseEnd {
		verb = "ended"
	}
	baselineOn, comparisonOn := e.installations()
	return fmt.Sprintf("benchmarkeks phase %d/%d %s: baseline on installation %s, comparison on installation %s", e.Phase+1, e.Phases, verb, baselineOn, comparisonOn)
}

// Text returns the text of the event, in Markdown, which the events API expects between a
// "%%% \n" and a "\n %%%".
func (e PhaseEvent) Text() string {
	baselineOn, comparisonOn := e.installations()
	var b strings.Builder
	b.WriteString("%%% \n")
	fmt.Fprintf(&b, "Measurement phase %d of %d, %s:\n\n", e.Phase+1, e.Phases, e.Window.span())
	fmt.Fprintf(&b, "- installation %s runs the baseline images: %s\n", baselineOn, codeList(e.Baseline))
	fmt.Fprintf(&b, "- installation %s runs the comparison images: %s\n", comparisonOn, codeList(e.Comparison))
	b.WriteString(" %%%")
	return b.String()
}

// Tags returns the tags of the event that tell the phases and their boundaries apart: the
// dashboard of the benchmark selects the events by the benchmark_phase_boundary tag.
func (e PhaseEvent) Tags() []string {
	return []string{
		"benchmark_phase:" + strconv.Itoa(e.Phase+1),
		"benchmark_phase_boundary:" + string(e.Boundary),
	}
}

// codeList formats values as a comma-separated list of code spans.
func codeList(values []string) string {
	return strings.Join(lo.Map(values, func(value string, _ int) string { return "`" + value + "`" }), ", ")
}
