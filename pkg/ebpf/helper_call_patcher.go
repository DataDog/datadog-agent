// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/asm"
)

var noopIns = asm.Mov.Reg(asm.R1, asm.R1)

// NewHelperCallRemover provides a `Modifier` that patches the eBPF bytecode
// replacing helper calls by NO-OP operations
func NewHelperCallRemover(helpers ...asm.BuiltinFunc) Modifier {
	return &helperCallRemover{
		helpers: helpers,
	}
}

type helperCallRemover struct {
	helpers []asm.BuiltinFunc
}

func (h *helperCallRemover) BeforeInit(m *manager.Manager, _ *manager.Options) error {
	m.InstructionPatchers = append(m.InstructionPatchers, func(m *manager.Manager) error {
		progs, err := m.GetProgramSpecs()
		if err != nil {
			return err
		}

		for _, p := range progs {
			iter := p.Instructions.Iterate()

		InstructionLoop:
			for iter.Next() {
				ins := iter.Ins
				if !ins.IsBuiltinCall() {
					continue
				}

				for _, fn := range h.helpers {
					if ins.Constant == int64(fn) {
						*ins = noopIns.WithMetadata(ins.Metadata)
						continue InstructionLoop
					}
				}
			}
		}

		return nil
	})

	return nil
}

func (h *helperCallRemover) AfterInit(*manager.Manager, *manager.Options) error {
	return nil
}

func (h *helperCallRemover) String() string {
	return fmt.Sprintf("HelperCallRemover[%+v]", h.helpers)
}
