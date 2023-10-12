// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cilium/ebpf"
)

// availableFunctionsBasedExcluder is a FunctionExcluder based on reading entries in
// `/sys/kernel/{debug,}/tracing/available_filter_functions`, i.e. the list of hookable
// functions provided by the kernel
type availableFunctionsBasedExcluder struct {
	available map[string]struct{}
}

var _ manager.FunctionExcluder = (*availableFunctionsBasedExcluder)(nil)

func newAvailableFunctionsBasedExcluder() (*availableFunctionsBasedExcluder, error) {
	tracingRoot, err := tracefs.Root()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filepath.Join(tracingRoot, "available_filter_functions"))
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

	seclog.Debugf("available functions excluder: entry count: %d\n", len(available))

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
