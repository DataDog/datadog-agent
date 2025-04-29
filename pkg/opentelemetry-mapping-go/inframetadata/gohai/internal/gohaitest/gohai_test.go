// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package gohaitest

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/gohai"
	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/filesystem"
	"github.com/DataDog/gohai/memory"
	"github.com/DataDog/gohai/network"
	"github.com/DataDog/gohai/platform"
	"github.com/stretchr/testify/require"
)

func TestGohaiHasTheRightTypes(t *testing.T) {
	res := new(gohai.Gohai)
	p, err := new(cpu.Cpu).Collect()
	require.NoError(t, err)
	res.CPU = p.(map[string]string)
	require.NotNil(t, res.CPU)

	p, err = new(filesystem.FileSystem).Collect()
	require.NoError(t, err)
	res.FileSystem = p.([]any)
	require.NotNil(t, res.FileSystem)

	p, err = new(memory.Memory).Collect()
	require.NoError(t, err)
	res.Memory = p.(map[string]string)
	require.NotNil(t, res.Memory)

	p, err = new(network.Network).Collect()
	require.NoError(t, err)
	res.Network = p.(map[string]any)
	require.NotNil(t, res.Network)

	p, err = new(platform.Platform).Collect()
	require.NoError(t, err)
	res.Platform = p.(map[string]string)
	require.NotNil(t, res.Platform)
}
