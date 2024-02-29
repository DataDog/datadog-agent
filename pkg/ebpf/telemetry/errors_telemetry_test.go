// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

func instrumentationEnabled(t *testing.T, dir, filename string) {
	buf, err := bytecode.GetReader(dir, filename)
	require.NoError(t, err)
	t.Cleanup(func() { _ = buf.Close })

	instrumented, err := elfBuildWithInstrumentation(buf)
	require.NoError(t, err)
	if !instrumented {
		t.Skip("Skipping because prebuilt and co-re assets are not instrumented")
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(buf)
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
}

func TestBinaryCorrectlyInstrumented(t *testing.T) {
	bpfDir := os.Getenv("DD_SYSTEM_PROBE_BPF_DIR")
	require.True(t, bpfDir != "")

	t.Run("usm/co-re", func(t *testing.T) {
		instrumentationEnabled(t, filepath.Join(bpfDir, "co-re"), "usm.o")
	})
	t.Run("usm/prebuilt", func(t *testing.T) {
		instrumentationEnabled(t, bpfDir, "usm.o")
	})
	t.Run("tracer/co-re", func(t *testing.T) {
		instrumentationEnabled(t, filepath.Join(bpfDir, "co-re"), "tracer.o")
	})
	t.Run("tracer/prebuilt", func(t *testing.T) {
		instrumentationEnabled(t, bpfDir, "tracer.o")
	})
	t.Run("shared-libraries/co-re", func(t *testing.T) {
		instrumentationEnabled(t, filepath.Join(bpfDir, "co-re"), "shared-libraries.o")
	})
	t.Run("shared-libraries/prebuilt", func(t *testing.T) {
		instrumentationEnabled(t, bpfDir, "shared-libraries.o")
	})
	t.Run("tracer-fentry/co-re", func(t *testing.T) {
		instrumentationEnabled(t, filepath.Join(bpfDir, "co-re"), "tracer-fentry.o")
	})
	// Fentry based tracer is only built in CO-RE mode
}
