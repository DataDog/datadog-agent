// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gnmistatusimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/status"
)

func TestNewComponentSharesRegistryBetweenOutputs(t *testing.T) {
	provides := NewComponent(Requires{})

	require.NotNil(t, provides.Comp)
	require.NotNil(t, provides.StatusProvider.Provider)
	assert.Equal(t, "GNMI", provides.StatusProvider.Provider.Name())

	provides.Comp.RegisterDevice("10.0.0.1", 57400, "basic-interfaces", false, nil)
	provides.Comp.UpdateDevice("10.0.0.1", 57400, func(device *status.DeviceState) {
		device.Started = true
		device.StreamState = "connected"
	})

	stats := make(map[string]interface{})
	require.NoError(t, provides.StatusProvider.Provider.JSON(false, stats))

	devices, ok := stats["devices"].([]status.DeviceState)
	require.True(t, ok)
	require.Len(t, devices, 1)
	assert.Equal(t, "10.0.0.1", devices[0].Address)
	assert.Equal(t, "connected", devices[0].StreamState)
}
