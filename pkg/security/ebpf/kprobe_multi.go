// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"fmt"
	"os"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// kprobeMultiBackend is an opt-in research backend using one multi-kprobe link
// per existing hook. It preserves the programs, hook targets and selectors.
// If the kernel capability is unavailable, initialization keeps the legacy
// program specifications and attachments.
type kprobeMultiBackend struct {
	checkSupport func() error
}

func (*kprobeMultiBackend) String() string { return "KprobeMultiBackend" }

func (b *kprobeMultiBackend) BeforeInit(m *manager.Manager, _ names.ModuleName, _ *manager.Options) error {
	if os.Getenv("DD_CWS_KPROBE_MULTI_EXPERIMENT") != "true" {
		return nil
	}
	checkSupport := b.checkSupport
	if checkSupport == nil {
		checkSupport = checkKprobeMultiSupport
	}
	if err := checkSupport(); err != nil {
		// Decide before retargeting any callers or tail-call targets. Falling back
		// after loading multi-kprobe programs would require reloading the graph.
		seclog.Warnf("CWS multi-kprobe backend unavailable; falling back to legacy kprobe attachments: %v", err)
		return nil
	}
	specs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}
	retargetKprobePrograms(specs)
	seclog.Infof("CWS multi-kprobe backend selected")
	return nil
}

func checkKprobeMultiSupport() error {
	if err := features.HaveBPFLinkKprobeMulti(); err != nil {
		return fmt.Errorf("CWS multi-kprobe backend unsupported: %w", err)
	}
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
