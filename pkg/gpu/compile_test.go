// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
)

func TestGPUCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		if err := config.CheckGPUSupported(); err != nil {
			t.Skip("GPU Runtime compilation not supported on this kernel version")
		}

		cfg := config.New()
		cfg.BPFDebug = true
		out, err := getRuntimeCompiledGPUMonitoring(cfg)
		require.NoError(t, err)
		_ = out.Close()
	})
}
