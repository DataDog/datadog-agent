// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package loader

import (
	"fmt"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

// TestRelocationAreOnlyPtRegs tests that the relocation metadata is only
// present for pt_regs offsets.
func TestRelocationAreOnlyPtRegs(t *testing.T) {
	// The relocation metadata doesn't have a rich API but it has a nice
	// String() method we can use to introspect the contents and validate that
	// it matches our expectations.
	//
	// For different architectures there's a different index path because of
	// the structure of the pt_regs struct.
	const reloRegex = `CORERelocation\(byte_off, ` +
		`Struct:"pt_regs"\[0(:[[:digit:]]+)+\], ` +
		`local_id=[[:digit:]]+\)`
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
