// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"errors"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// patchPrintkNewline patches log_debug calls to always print one newline, no matter what the kernel does.
//
// For context, in kernel 5.9.0, bpf_trace_printk adds a newline automatically to anything it prints
// This means that when we support both older and newer kernels, bpf_printk is going to have
// an inconsistent behavior. To avoid this, we have a wrapper called log_debug (see bpf_helpers_custom.h)
// that adds a newline to the message before calling bpf_trace_printk. In older kernels
// this ensures that a newline is added. In newer ones it would mean that two newlines are
// added, so this patcher removes that newline in those cases.
func patchPrintkNewline(m *manager.Manager) error {
	kernelVersion, err := kernel.HostVersion()
	if err != nil {
		return err // can't detect kernel version, don't patch
	}
	if kernelVersion < kernel.VersionCode(5, 9, 0) {
		return nil // Do nothing in older kernels
	}

	progs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}

	var errs []error

	for _, p := range progs {
		patcher := newPrintkPatcher(p)
		_, err := patcher.patch()
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// PrintkPatcherModifier adds an InstructionPatcher to the manager that removes the newline character from log_debug calls if needed
type PrintkPatcherModifier struct {
}

// ensure PrintkPatcherModifier implements the ModifierBeforeInit interface
var _ ModifierBeforeInit = &PrintkPatcherModifier{}

func (t *PrintkPatcherModifier) String() string {
	return "PrintkPatcherModifier"
}

// BeforeInit adds the patchPrintkNewline function to the manager
func (t *PrintkPatcherModifier) BeforeInit(m *manager.Manager, _ names.ModuleName, _ *manager.Options) error {
	m.InstructionPatchers = append(m.InstructionPatchers, patchPrintkNewline)
	return nil
}
