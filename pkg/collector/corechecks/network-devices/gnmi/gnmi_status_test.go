// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && gnmi

package gnmi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	gnmiStatus "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/status"
)

const (
	statusTestAddress = "10.0.0.1"
	statusTestPort    = 57400
)

// statusDevices returns the devices exposed by the status provider.
func statusDevices(t *testing.T, registry *gnmiStatus.Registry) []gnmiStatus.DeviceDisplay {
	t.Helper()
	stats := map[string]interface{}{}
	require.NoError(t, gnmiStatus.NewProvider(registry).JSON(false, stats))
	devices, ok := stats["devices"].([]gnmiStatus.DeviceDisplay)
	require.True(t, ok, "unexpected devices type %T", stats["devices"])
	return devices
}

// newStatusTestCheck returns a configured check whose device is registered in
// a fresh status registry, without starting a gNMI client.
func newStatusTestCheck(t *testing.T) (*Check, *gnmiStatus.Registry) {
	t.Helper()
	registry := gnmiStatus.NewRegistry()
	c := newCheck(registry).(*Check)
	c.config = &config.CheckConfig{Instance: config.InstanceConfig{Address: statusTestAddress, Port: statusTestPort}}
	registry.RegisterDevice(statusTestAddress, statusTestPort, "basic-interfaces", false, nil)
	require.Len(t, statusDevices(t, registry), 1)
	return c, registry
}

func TestRunRecordsClientStartErrorInStatus(t *testing.T) {
	c, registry := newStatusTestCheck(t)

	// A client that was started and then closed cannot be started again, so
	// Run fails before collecting.
	gnmiClient, err := client.New(client.Config{
		Address:  statusTestAddress,
		Port:     statusTestPort,
		Username: "user",
		Password: "pass",
		Profile:  config.ProfileDefinition{Metrics: []config.MetricConfig{{Path: "/system/state/uptime", Metric: "snmp.sysUpTime"}}},
	})
	require.NoError(t, err)
	require.NoError(t, gnmiClient.Start(context.Background()))
	require.NoError(t, gnmiClient.Close())
	c.client = gnmiClient

	runErr := c.Run()
	require.Error(t, runErr)

	devices := statusDevices(t, registry)
	require.Len(t, devices, 1)
	assert.Equal(t, runErr.Error(), devices[0].LastError)
	assert.NotZero(t, devices[0].LastErrorAt)
}

func TestUpdateStatusLastErrorIgnoresNilErrorAndUnconfiguredCheck(t *testing.T) {
	c, registry := newStatusTestCheck(t)

	c.updateStatusLastError(nil)
	assert.Empty(t, statusDevices(t, registry)[0].LastError)

	c.config = nil
	c.updateStatusLastError(assert.AnError)
	assert.Empty(t, statusDevices(t, registry)[0].LastError)
}

func TestCancelUnregistersDevice(t *testing.T) {
	c, registry := newStatusTestCheck(t)

	c.Cancel()

	assert.Empty(t, statusDevices(t, registry))
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Nil(t, c.config)
	assert.Nil(t, c.client)
}
