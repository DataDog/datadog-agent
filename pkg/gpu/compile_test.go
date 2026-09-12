// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package gpu

import (
	"testing"

	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
)

func TestGPUCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		if err := checkGPUSupported(); err != nil {
			t.Skip("GPU Runtime compilation not supported on this kernel version")
		}

		ebpfCfg := ddebpf.NewConfig()
		ebpfCfg.BPFDebug = true
		out, err := getRuntimeCompiledGPUMonitoring(ebpfCfg)
		require.NoError(t, err)
		_ = out.Close()
	})
}
