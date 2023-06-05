// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"testing"

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
