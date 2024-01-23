// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

// Package verifier is responsible for exposing information the verifier provides
// for any loaded eBPF program
package verifier

import "fmt"

type Statistics struct{}

func BuildVerifierStats(objectFiles []string) (map[string]*Statistics, error) {
	return nil, fmt.Errorf("not implemented")
}
