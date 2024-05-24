// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package verifier is responsible for exposing information the verifier provides
// for any loaded eBPF program
package verifier

import "regexp"

// stats holds the value of a verifier statistics and a regular expression
// to parse it from the verifier log.
type stat struct {
	// `Value` must be exported to be settable
	Value int
	parse *regexp.Regexp //nolint:unused
}

// Statistics represent that statistics exposed via
// the eBPF verifier when  LogLevelStats is enabled
type Statistics struct {
	StackDepth                 stat `json:"stack_usage" kernel:"4.15"`
	InstructionsProcessed      stat `json:"instruction_processed" kernel:"4.15"`
	InstructionsProcessedLimit stat `json:"limit" kernel:"4.15"`
	MaxStatesPerInstruction    stat `json:"max_states_per_insn" kernel:"5.2"`
	TotalStates                stat `json:"total_states" kernel:"5.2"`
	PeakStates                 stat `json:"peak_states" kernel:"5.2"`
}

// SourceLine holds the information about a C source line
type SourceLine struct {
	LineInfo string `json:"line_info"`
	Line     string `json:"line"`
}

// RegisterState holds the state for a given register
type RegisterState struct {
	Register int    `json:"register"`
	Live     string `json:"live"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	Precise  bool   `json:"precise"`
}

// InstructionInfo holds information about an eBPF instruction extracted from the verifier
type InstructionInfo struct {
	Index            int                    `json:"index"`
	TimesProcessed   int                    `json:"times_processed"`
	Source           *SourceLine            `json:"source"`
	Code             string                 `json:"code"`
	RegisterState    map[int]*RegisterState `json:"register_state"`
	RegisterStateRaw string                 `json:"register_state_raw"`
}

// SourceLineStats holds the aggregate verifier statistics for a given C source line
type SourceLineStats struct {
	NumInstructions            int   `json:"num_instructions"`
	MaxPasses                  int   `json:"max_passes"`
	MinPasses                  int   `json:"min_passes"`
	TotalInstructionsProcessed int   `json:"total_instructions_processed"`
	AssemblyInsns              []int `json:"assembly_insns"`
}

// ComplexityInfo holds the complexity information for a given eBPF program, with assembly
// and source line information
type ComplexityInfo struct {
	InsnMap   map[int]*InstructionInfo    `json:"insn_map"`
	SourceMap map[string]*SourceLineStats `json:"source_map"`
}

// StatsOptions holds the options for the function BuildVerifierStats
type StatsOptions struct {
	ObjectFiles        []string
	FilterPrograms     []*regexp.Regexp
	DetailedComplexity bool
	VerifierLogsDir    string
}

// StatsResult holds the result of the verifier stats process
type StatsResult struct {
	Stats           map[string]*Statistics         // map of program name to statistics
	Complexity      map[string]*ComplexityInfo     // map of program name to complexity info
	FuncsPerSection map[string]map[string][]string // map of object name to the list of functions per section
}
