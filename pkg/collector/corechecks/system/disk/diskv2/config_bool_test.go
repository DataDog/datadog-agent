// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diskv2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// yaml v3 still resolves YAML 1.1 booleans in typed bool fields, so these options keep accepting them.
func TestConfigureYAML11BooleanSpellings(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("disk_check.use_core_loader", true, configmodel.SourceAgentRuntime)

	newCheck := func() *Check {
		opt := Factory()
		factory, _ := opt.Get()
		return factory().(*Check)
	}

	c := newCheck()
	require.NoError(t, c.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash,
		integration.Data("use_mount: yes\nall_partitions: on\nlowercase_device_tag: Y\ntag_by_label: no\n"),
		integration.Data(""), "test", ""))
	assert.True(t, c.instanceConfig.UseMount)
	assert.True(t, c.instanceConfig.AllPartitions)
	assert.True(t, c.instanceConfig.LowercaseDeviceTag)
	// TagByLabel defaults to true, so this pins the false direction
	assert.False(t, c.instanceConfig.TagByLabel)

	// The typed unmarshaling rejects string "true", like the disk v1 map read accepts it.
	c = newCheck()
	require.Error(t, c.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash,
		integration.Data("use_mount: \"true\"\n"),
		integration.Data(""), "test", ""))

	c = newCheck()
	require.NoError(t, c.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash,
		integration.Data("use_mount: true\n"),
		integration.Data(""), "test", ""))
	assert.True(t, c.instanceConfig.UseMount)

	// Non-boolean values fail the configuration load.
	c = newCheck()
	require.Error(t, c.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash,
		integration.Data("use_mount: maybe\n"),
		integration.Data(""), "test", ""))
}
