// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"errors"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ddbtf "github.com/DataDog/datadog-agent/pkg/ebpf/btf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// syscallAliasesModifier attaches the fentry and fexit programs of the syscall wrappers that have no BTF to the
// function they alias.
//
// On x86, the wrappers of the syscalls without argument, like __x64_sys_setsid, are aliases of __do_sys_<name>.
// pahole doesn't generate BTF for aliases, so the tracing programs can't be attached to them, and the hook points
// using a best effort selector are silently skipped. __do_sys_<name> takes the same pt_regs pointer argument.
type syscallAliasesModifier struct {
	kernelTypes func() (*btf.Spec, error)
}

var _ ddebpf.ModifierBeforeInit = &syscallAliasesModifier{}

func newSyscallAliasesModifier() *syscallAliasesModifier {
	return &syscallAliasesModifier{kernelTypes: ddbtf.GetKernelSpec}
}

func (s *syscallAliasesModifier) String() string {
	return "SyscallAliasesModifier"
}

// BeforeInit retargets the programs before the excluded functions, the constants and the tail call groups are computed
// from their attach point
func (s *syscallAliasesModifier) BeforeInit(m *manager.Manager, _ names.ModuleName, _ *manager.Options) error {
	prefix := probes.GetSyscallFnName("")
	if prefix == "sys_" || !strings.HasSuffix(prefix, "sys_") {
		return nil
	}

	progSpecs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}

	retargetSyscallAliases(progSpecs, prefix, s.kernelTypes)
	return nil
}

func retargetSyscallAliases(progSpecs map[string]*lib.ProgramSpec, prefix string, loadKernelSpec func() (*btf.Spec, error)) {
	var kernelSpec *btf.Spec
	for name, spec := range progSpecs {
		if spec.Type != lib.Tracing || !strings.HasPrefix(spec.AttachTo, prefix) {
			continue
		}

		if kernelSpec == nil {
			var err error
			if kernelSpec, err = loadKernelSpec(); err != nil {
				seclog.Debugf("failed to load the kernel BTF to resolve the syscall aliases: %v", err)
				return
			}
		}

		var fn *btf.Func
		if err := kernelSpec.TypeByName(spec.AttachTo, &fn); !errors.Is(err, btf.ErrNotFound) {
			continue
		}

		target := "__do_sys_" + strings.TrimPrefix(spec.AttachTo, prefix)
		if err := kernelSpec.TypeByName(target, &fn); err != nil || !takesPtRegs(fn) {
			continue
		}

		seclog.Debugf("attaching %s to %s, %s has no BTF", name, target, spec.AttachTo)
		spec.AttachTo = target
	}
}

// takesPtRegs returns whether the only argument of the function is a pointer to a pt_regs structure, like a syscall
// wrapper
func takesPtRegs(fn *btf.Func) bool {
	proto, ok := fn.Type.(*btf.FuncProto)
	if !ok || len(proto.Params) != 1 {
		return false
	}

	ptr, ok := btf.UnderlyingType(proto.Params[0].Type).(*btf.Pointer)
	if !ok {
		return false
	}

	st, ok := btf.UnderlyingType(ptr.Target).(*btf.Struct)
	return ok && st.Name == "pt_regs"
}
