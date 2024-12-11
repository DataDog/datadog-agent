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
)

// BuildVerifierStats accepts a list of eBPF object files and generates a
// map of all programs and their Statistics
func BuildVerifierStats(_ *StatsOptions) (*StatsResult, map[string]struct{}, error) {
	return nil, nil, fmt.Errorf("not implemented")
}
