// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"errors"
	"fmt"
	"os"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
)

// kprobeMultiBackend is an opt-in research backend using one multi-kprobe link
// per existing hook. It preserves the programs, hook targets and selectors.
// The companion ebpf-manager change is required; kernel compatibility and
// fallback validation remain prerequisites for production enablement.
type kprobeMultiBackend struct{}

func (*kprobeMultiBackend) String() string { return "KprobeMultiBackend" }

func (*kprobeMultiBackend) BeforeInit(m *manager.Manager, _ names.ModuleName, _ *manager.Options) error {
	if os.Getenv("DD_CWS_KPROBE_MULTI_EXPERIMENT") != "true" {
		return nil
	}
	backend, ok := any(&manager.Probe{}).(interface{ SupportsKprobeMulti() bool })
	if !ok || !backend.SupportsKprobeMulti() {
		return errors.New("CWS multi-kprobe backend requires the companion ebpf-manager patch")
	}
	if err := features.HaveBPFLinkKprobeMulti(); err != nil {
		return fmt.Errorf("CWS multi-kprobe backend unsupported: %w", err)
	}
	specs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}
	retargetKprobePrograms(specs)
	return nil
}

func retargetKprobePrograms(specs map[string]*lib.ProgramSpec) {
	for _, spec := range specs {
		// Tail-call targets must use the same expected attach type as their callers.
		// Include unattached kprobe tail-call targets, while preserving uprobes,
		// tracepoints and fentry/fexit programs and their separate program arrays.
		if spec.Type == lib.Kprobe && (strings.HasPrefix(spec.SectionName, "kprobe/") || strings.HasPrefix(spec.SectionName, "kretprobe/")) {
			spec.AttachType = lib.AttachTraceKprobeMulti
		}
	}
}
