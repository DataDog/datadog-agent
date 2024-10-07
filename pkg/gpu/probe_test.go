// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestProbeCanLoad(t *testing.T) {
	kver, err := kernel.HostVersion()
	require.NoError(t, err)
	if kver < minimumKernelVersion {
		t.Skipf("minimum kernel version %s not met, read %s", minimumKernelVersion, kver)
	}

	probe, err := NewProbe(NewConfig(), nil)
	require.NoError(t, err)
	require.NotNil(t, probe)
	t.Cleanup(probe.Close)

	data, err := probe.GetAndFlush()
	require.NoError(t, err)
	require.NotNil(t, data)
}
