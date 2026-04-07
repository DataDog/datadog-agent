// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package modifiers

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type SleepableProgramModifier struct {
	ProbeIDs []manager.ProbeIdentificationPair
}

var _ ddebpf.ModifierBeforeInit = &SleepableProgramModifier{}

func (t *SleepableProgramModifier) String() string {
	return "SleepableProgramModifier"
}

func isProgramSleepable(prog *ebpf.ProgramSpec) bool {
	if prog.Type == ebpf.Tracing {
		switch prog.AttachType {
		case ebpf.AttachTraceFEntry:
			fallthrough
		case ebpf.AttachTraceFExit:
			fallthrough
		case ebpf.AttachModifyReturn:
			fallthrough
		case ebpf.AttachTraceIter:
			return true
		default:
			return false
		}
	}

	return prog.Type == ebpf.LSM || prog.Type == ebpf.Kprobe || prog.Type == ebpf.StructOps
}

func (t *SleepableProgramModifier) BeforeInit(m *manager.Manager, module names.ModuleName, opts *manager.Options) error {
	helperCallReplacers := []HelperReplacer{}
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
			if !isProgramSleepable(spec) {
				return fmt.Errorf("program %s of type %v and attach type %v is not sleepable", spec.Name, spec.Type, spec.AttachType)
			}

			spec.Flags |= unix.BPF_F_SLEEPABLE

			helperCallReplacers = append(helperCallReplacers, HelperReplacer{
				Target: asm.FnProbeReadUser,
				New:    asm.FnCopyFromUser,
				Prog:   spec,
			})
		}
	}

	// we cannot use perf events with sleepable programs so remove this helper calls
	// if there are no programs to make sleepable, remove any references to
	// bpf_copy_from_user
	if len(t.ProbeIDs) > 0 {
		patcher := NewHelperCallRemover(asm.FnPerfEventOutput)
		err := patcher.BeforeInit(m, module, opts)
		if err != nil {
			return fmt.Errorf("error patching helper calls from sleepable modifier on %q: %w", module.Name(), err)
		}

		replacer := NewHelperCallReplacer(helperCallReplacers...)
		err = replacer.BeforeInit(m, module, opts)
		if err != nil {
			return fmt.Errorf("error replacing probe_read_user with copy_from_user helpers: %w", err)
		}
	}

	return nil
}
