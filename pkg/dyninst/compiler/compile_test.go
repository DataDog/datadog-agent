// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var MinimumKernelVersion = kernel.VersionCode(5, 17, 0)

func skipIfKernelNotSupported(t *testing.T) {
	curKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if curKernelVersion < MinimumKernelVersion {
		t.Skipf("Kernel version %v is not supported", curKernelVersion)
	}
	if runtime.GOARCH != "amd64" {
		t.Skipf("platform %v is not supported", runtime.GOARCH)
	}
}

func TestCompileBPFProgram(t *testing.T) {
	skipIfKernelNotSupported(t)

	err := CompileBPFProgram()
	if err != nil {
		t.Fatalf("Failed to compile BPF program: %v", err)
	}
}
