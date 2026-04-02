// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/asm"
)

type SleepableProgramModifier struct {
	ProbeIDs      []manager.ProbeIdentificationPair
	SleepableMask uint64
}

var _ ModifierBeforeInit = &SleepableProgramModifier{}

const (
	BPF_F_SLEEPABLE = 1 << 4
)

func (t *SleepableProgramModifier) String() string {
	return "SleepableProgramModifier"
}

func (t *SleepableProgramModifier) BeforeInit(m *manager.Manager, module names.ModuleName, opts *manager.Options) error {
	for _, id := range t.ProbeIDs {
		// passing in a ProbeIdentificationPair with UID set forces the ebpf manager to return the ebpf.ProgramSpec associated with a manager.Probe
		// which is not set at this stage. When just the function name is passed the manager looks inside ebpf.CollectionSpec
		// which is intialized before the call to this function by manager.LoadELF
		specs, found, err := m.GetProgramSpec(manager.ProbeIdentificationPair{EBPFFuncName: id.EBPFFuncName})
		if err != nil {
			return fmt.Errorf("error getting program spec for probe with id %v in sleepable modifier: %w", id, err)
		}
		if !found {
			return fmt.Errorf("program spec for probe with id %v not found", id)
		}

		log.Infof("Marking %s sleepable as requested by %q", id.EBPFFuncName, module.Name())
		for _, spec := range specs {
			spec.Flags |= BPF_F_SLEEPABLE
		}
	}

	if len(t.ProbeIDs) > 0 {
		opts.ConstantEditors = append(opts.ConstantEditors, manager.ConstantEditor{
			Name:  "sleepable_" + module.Name(),
			Value: t.SleepableMask,
		})
	}

	// we cannot use perf events with sleepable programs so remove this helper calls
	patcher := NewHelperCallRemover(asm.FnPerfEventOutput)
	// if there are no programs to make sleepable, remove any references to
	// bpf_copy_from_user
	if len(t.ProbeIDs) == 0 {
		patcher = NewHelperCallRemover(asm.FnCopyFromUser)
	}

	err := patcher.BeforeInit(m, module, opts)
	if err != nil {
		return fmt.Errorf("error patching helper calls from sleepable modifier on %q: %w", module.Name(), err)
	}

	return nil
}
