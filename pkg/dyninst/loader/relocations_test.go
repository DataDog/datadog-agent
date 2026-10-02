// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package loader

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

// TestRelocationAreOnlyPtRegs tests that the relocation metadata is only
// present for pt_regs fields that every kernel lays out the same way.
//
// The loader strips all relocations before loading, so the offsets baked
// into the object at compile time are the ones the kernel sees. That is
// only safe for fields whose offset is fixed by the architecture's
// user-space ABI. The tail of arm64's struct pt_regs is kernel-internal
// and does change: Linux 7.0 dropped two fields from it, and the verifier
// rejects any context read past the running kernel's sizeof(struct
// pt_regs).
func TestRelocationAreOnlyPtRegs(t *testing.T) {
	// The relocation metadata doesn't have a rich API but it has a nice
	// String() method we can use to introspect the contents and validate that
	// it matches our expectations.
	//
	// The index path is the chain of member/array indexes from the start of
	// the struct, so constraining its prefix constrains which part of
	// pt_regs we touch.
	var fieldPath string
	switch runtime.GOARCH {
	case "arm64":
		// Member 0 of arm64's pt_regs is the union holding struct
		// user_pt_regs (x0-x30, sp, pc, pstate), which is UAPI. Everything
		// after it is kernel-internal.
		fieldPath = `0:0(:[[:digit:]]+)+`
	default:
		// x86-64's pt_regs is the ptrace layout in full, with no
		// kernel-internal tail.
		fieldPath = `0(:[[:digit:]]+)+`
	}
	reloRegex := `^CORERelocation\(byte_off, ` +
		`Struct:"pt_regs"\[` + fieldPath + `\], ` +
		`local_id=[[:digit:]]+\)$`
	for _, debug := range []bool{true, false} {
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			cfg := &config{
				dyninstDebugEnabled: debug,
				ebpfConfig:          ddebpf.NewConfig(),
			}
			obj, err := getBpfObject(cfg)
			require.NoError(t, err)
			defer obj.Close()

			spec, err := ebpf.LoadCollectionSpecFromReader(obj)
			require.NoError(t, err)
			for _, p := range spec.Programs {
				for _, insn := range p.Instructions {
					relo := btf.CORERelocationMetadata(&insn)
					if relo == nil {
						continue
					}
					require.Regexp(t, reloRegex, relo.String())
				}
			}
		})
	}
}
