// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

// Package verifier is responsible for exposing information the verifier provides
// for any loaded eBPF program
package verifier

import (
	"fmt"
	"regexp"
)

// Statistics represent that statistics exposed via
// the eBPF verifier when  LogLevelStats is enabled
type Statistics struct {
	VerificationTime           int `json:"verification_time"`
	StackDepth                 int `json:"stack_usage"`
	InstructionsProcessed      int `json:"instruction_processed"`
	InstructionsProcessedLimit int `json:"limit"`
	MaxStatesPerInstruction    int `json:"max_states_per_insn"`
	TotalStates                int `json:"total_states"`
	PeakStates                 int `json:"peak_states"`
}

// BuildVerifierStats accepts a list of eBPF object files and generates a
// map of all programs and their Statistics
func BuildVerifierStats(_ []string, _ []*regexp.Regexp) (map[string]*Statistics, error) {
	return nil, fmt.Errorf("not implemented")
}
