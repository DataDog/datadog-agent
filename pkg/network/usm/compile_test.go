// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"fmt"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestHttpCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		currKernelVersion, err := kernel.HostVersion()
		require.NoError(t, err)
		if currKernelVersion < http.MinimumKernelVersion {
			t.Skip("USM Runtime compilation not supported on this kernel version")
		}
		cfg := config.New()
		cfg.BPFDebug = true
		out, err := getRuntimeCompiledUSM(cfg)
		require.NoError(t, err)
		_ = out.Close()
	})
}

func TestUSMCorrectlyInstrumentedWithTrampoline(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		currKernelVersion, err := kernel.HostVersion()
		require.NoError(t, err)
		if currKernelVersion < http.MinimumKernelVersion {
			t.Skip("USM Runtime compilation not supported on this kernel version")
		}
		cfg := config.New()
		cfg.EBPFInstrumentationEnabled = true
		out, err := getRuntimeCompiledUSM(cfg)
		require.NoError(t, err)
		t.Cleanup(func() { _ = out.Close() })

		spec, err := ebpf.LoadCollectionSpecFromReader(out)
		require.NoError(t, err)

		const ebpfEntryTrampolinePatchCall = -1
		const maxTrampolineOffset = 2
		for _, prog := range spec.Programs {
			iter := prog.Instructions.Iterate()
			found := false
			for iter.Next() {
				ins := iter.Ins
				if iter.Offset > maxTrampolineOffset {
					// The trampoline instruction should be discovered at most within two instructions
					require.True(t, false, fmt.Sprintf("EBPF trampoline not found within offset of %d instructions", maxTrampolineOffset))
				}

				if ins.OpCode.JumpOp() == asm.Call && ins.Constant == ebpfEntryTrampolinePatchCall && iter.Offset <= maxTrampolineOffset {
					found = true
					break
				}
			}
			require.True(t, found, "EBPF trampoline not found")
		}
	})
}
