// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package gohaitest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/gohai/filesystem"
	"github.com/DataDog/datadog-agent/pkg/gohai/memory"
	"github.com/DataDog/datadog-agent/pkg/gohai/network"
	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/inframetadata/gohai"
)

func TestGohaiHasTheRightTypes(t *testing.T) {
	res := new(gohai.Gohai)

	p, _, err := cpu.CollectInfo().AsJSON()
	require.NoError(t, err)
	res.CPU = p.(map[string]any)
	require.NotNil(t, res.CPU)

	info, err := filesystem.CollectInfo()
	require.NoError(t, err)
	p, _, err = info.AsJSON()
	require.NoError(t, err)
	res.FileSystem = p.([]any)
	require.NotNil(t, res.FileSystem)

	p, _, err = memory.CollectInfo().AsJSON()
	require.NoError(t, err)
	res.Memory = p.(map[string]any)
	require.NotNil(t, res.Memory)

	info2, err := network.CollectInfo()
	require.NoError(t, err)
	p, _, err = info2.AsJSON()
	require.NoError(t, err)
	res.Network = p.(map[string]any)
	require.NotNil(t, res.Network)

	p, _, err = platform.CollectInfo().AsJSON()
	require.NoError(t, err)
	res.Platform = p.(map[string]any)
	require.NotNil(t, res.Platform)
}
