// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package uprobes

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

type multiAttachment struct {
	link    io.Closer
	offsets []uint64
}

type multiProbeKey struct {
	program string
	ret     bool
}

// UseUprobeMultiPrograms selects the multi-uprobe backend. Programs must have
// AttachTraceUprobeMulti and remain alive until after Stop. Call before Start;
// the attacher owns its links, but not these shared programs or their maps.
func (ua *UprobeAttacher) UseUprobeMultiPrograms(programs map[string]*ebpf.Program) {
	ua.multiPrograms = programs
}

func attachUprobeMulti(path string, program *ebpf.Program, offsets []uint64, ret bool) (io.Closer, error) {
	executable, err := link.OpenExecutable(path)
	if err != nil {
		return nil, err
	}
	opts := &link.UprobeMultiOptions{Addresses: offsets}
	if ret {
		return executable.UretprobeMulti(nil, program, opts)
	}
	return executable.UprobeMulti(nil, program, opts)
}

func (ua *UprobeAttacher) attachMultiProbeSelector(selector manager.ProbesSelector, path utils.FilePath, rule *AttachRule, symbols map[string]bininspect.FunctionMetadata) error {
	// Every requested group is attached synchronously below. A successful call
	// therefore satisfies AllOf/OneOf/BestEffort without manufacturing single
	// manager.Probe objects for links owned by this attacher.
	_, bestEffort := selector.(*manager.BestEffort)
	for _, id := range selector.GetProbesIdentificationPairList() {
		opts, err := rule.getProbeOptions(id)
		if err != nil {
			return err
		}
		symbol, ok := symbols[opts.Symbol]
		if !ok {
			if bestEffort {
				continue
			}
			return fmt.Errorf("symbol %s not found in %s", opts.Symbol, path.HostPath)
		}
		offsets := []uint64{symbol.EntryLocation}
		if opts.IsManualReturn {
			offsets = symbol.ReturnLocations
		}
		if len(offsets) == 0 {
			return fmt.Errorf("no probe locations for %s in %s", opts.Symbol, path.HostPath)
		}
		// Go's manual return locations are ordinary uprobes at return
		// instructions, not native uretprobes.
		key := multiProbeKey{program: id.EBPFFuncName, ret: !opts.IsManualReturn && strings.HasPrefix(id.EBPFFuncName, "uretprobe__")}
		groups := ua.fileIDToMultiLinks[path.ID]
		if attached, ok := groups[key]; ok {
			if !slices.Equal(attached.offsets, offsets) {
				return fmt.Errorf("conflicting locations for %s in %s", id.EBPFFuncName, path.HostPath)
			}
			continue
		}
		program := ua.multiPrograms[id.EBPFFuncName]
		if program == nil {
			return fmt.Errorf("missing multi-uprobe program %s", id.EBPFFuncName)
		}
		attached, err := ua.attachMulti(path.HostPath, program, offsets, key.ret)
		if err != nil {
			ua.telemetry.probeAttachErrorsAddHook.Inc()
			return fmt.Errorf("attach multi-uprobe %s: %w", id.EBPFFuncName, err)
		}
		if groups == nil {
			groups = make(map[multiProbeKey]multiAttachment)
			ua.fileIDToMultiLinks[path.ID] = groups
		}
		groups[key] = multiAttachment{link: attached, offsets: slices.Clone(offsets)}
		ua.telemetry.createdProbes.Add(float64(len(offsets)))
		ua.telemetry.attachedProbes.Add(float64(len(offsets)))
		ua.telemetry.createdMultiLinks.Inc()
	}
	return nil
}

func (ua *UprobeAttacher) detachMultiProbes(path utils.FilePath) error {
	var errs error
	for key, attached := range ua.fileIDToMultiLinks[path.ID] {
		// Attempt every close even when another one failed.
		if err := attached.link.Close(); err != nil {
			errs = errors.Join(errs, fmt.Errorf("detach multi-uprobe %s: %w", key.program, err))
		}
	}
	delete(ua.fileIDToMultiLinks, path.ID)
	return errs
}
