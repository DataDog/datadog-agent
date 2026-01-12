// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmp

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

func setupDevice(r *require.Assertions, vm *components.RemoteHost) {
	err := vm.CopyFolder("compose/data", "/tmp/data")
	r.NoError(err)

	vm.CopyFile("compose-vm/snmpCompose.yaml", "/tmp/snmpCompose.yaml")

	_, err = vm.Execute("docker-compose -f /tmp/snmpCompose.yaml up -d")
	r.NoError(err)
}

func checkBasicMetrics(c *assert.CollectT, fakeIntake *components.FakeIntake) {
	metrics, err := fakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metrics, "snmp.sysUpTimeInstance", "Metrics %v doesn't contain snmp.sysUpTimeInstance", metrics)
}

func checkLastNDMPayload(c *assert.CollectT, fakeIntake *components.FakeIntake, expectedNamespace string) *aggregator.NDMPayload {
	ndmPayloads, err := fakeIntake.Client().GetNDMPayloads()
	require.NoError(c, err)
	require.NotEmpty(c, ndmPayloads)

	ndmPayload := ndmPayloads[len(ndmPayloads)-1]
	assert.Equal(c, "snmp", ndmPayload.Integration)
	assert.Equal(c, expectedNamespace, ndmPayload.Namespace)
	assert.Greater(c, len(ndmPayload.Devices), 0)
	assert.Greater(c, len(ndmPayload.Interfaces), 0)

	return ndmPayload
}

func checkCiscoNexusDeviceMetadata(c *assert.CollectT, deviceMetadata aggregator.DeviceMetadata) {
	assert.Equal(c, "default:127.0.0.1", deviceMetadata.ID)
	assert.Contains(c, deviceMetadata.IDTags, "snmp_device:127.0.0.1")
	assert.Contains(c, deviceMetadata.IDTags, "device_namespace:default")
	assert.Contains(c, deviceMetadata.Tags, "snmp_profile:cisco-nexus")
	assert.Contains(c, deviceMetadata.Tags, "device_vendor:cisco")
	assert.Contains(c, deviceMetadata.Tags, "snmp_device:127.0.0.1")
	assert.Contains(c, deviceMetadata.Tags, "device_namespace:default")
	assert.Equal(c, "127.0.0.1", deviceMetadata.IPAddress)
	assert.Equal(c, int32(1), deviceMetadata.Status)
	assert.Equal(c, "Nexus-eu1.companyname.managed", deviceMetadata.Name)
	assert.Equal(c, "oxen acted but acted kept", deviceMetadata.Description)
	assert.Equal(c, "1.3.6.1.4.1.9.12.3.1.3.1.2", deviceMetadata.SysObjectID)
	assert.Equal(c, "but kept Jaded their but kept quaintly driving their", deviceMetadata.Location)
	assert.Equal(c, "cisco-nexus", deviceMetadata.Profile)
	assert.Equal(c, "cisco", deviceMetadata.Vendor)
	assert.Equal(c, "switch", deviceMetadata.DeviceType)
}
