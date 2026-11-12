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

// TestConfigureYAML11BooleanSpellings pins the compatibility the check keeps
// after moving from go.yaml.in/yaml/v3 to go.yaml.in/yaml/v4: yaml v4, like
// v3, still resolves the YAML 1.1 boolean spellings (yes/no/on/off/y/n) when
// unmarshaling into typed bool fields, so the disk check's boolean options
// keep accepting them. Fleet data shows customers relying on e.g.
// `use_mount: yes`.
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
		integration.Data("use_mount: yes\nall_partitions: on\ntag_by_filesystem: 'no'\nlowercase_device_tag: Y\n"),
		integration.Data(""), "test", ""))
	assert.True(t, c.instanceConfig.UseMount)
	assert.True(t, c.instanceConfig.AllPartitions)
	assert.False(t, c.instanceConfig.TagByFilesystem)
	assert.True(t, c.instanceConfig.LowercaseDeviceTag)

	c = newCheck()
	require.NoError(t, c.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash,
		integration.Data("use_mount: true\nall_partitions: false\n"),
		integration.Data(""), "test", ""))
	assert.True(t, c.instanceConfig.UseMount)
	assert.False(t, c.instanceConfig.AllPartitions)

	// Values that are not boolean spellings are rejected by the typed
	// unmarshaling, so the check configuration fails to load.
	c = newCheck()
	require.Error(t, c.Configure(mocksender.CreateDefaultDemultiplexer(t), integration.FakeConfigHash,
		integration.Data("use_mount: maybe\n"),
		integration.Data(""), "test", ""))
}
