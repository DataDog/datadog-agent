// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/cilium/ebpf"
)

type availableFunctionsBasedExcluder struct {
	available map[string]struct{}
}

func newAvailableFunctionsBasedExcluder() (*availableFunctionsBasedExcluder, error) {
	f, err := os.Open("/sys/kernel/debug/tracing/available_filter_functions")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	available := make(map[string]struct{})

	for scanner.Scan() {
		name, _, _ := strings.Cut(scanner.Text(), " ")
		available[name] = struct{}{}
	}

	fmt.Printf("size available: %d\n", len(available))

	return &availableFunctionsBasedExcluder{
		available: available,
	}, nil
}

func (af *availableFunctionsBasedExcluder) ShouldExcludeFunction(name string, prog *ebpf.ProgramSpec) bool {
	if af.available == nil {
		return false
	}

	if prog.Type != ebpf.Kprobe && prog.Type != ebpf.Tracing {
		return false
	}

	if strings.HasPrefix(name, "tail_call") || strings.Contains(name, "dentry_resolver") || strings.Contains(name, "callback") {
		return false
	}

	_, ok := af.available[prog.AttachTo]
	return !ok
}

func (af *availableFunctionsBasedExcluder) CleanCaches() {
	af.available = nil
}
