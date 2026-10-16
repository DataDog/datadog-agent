// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorypayload

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func newTestData(t *testing.T) *Data {
	hostname, _ := hostnamemock.NewMock("data-hostname")
	return NewData(config.NewMock(t), logmock.New(t), hostname)
}

func TestSet(t *testing.T) {
	d := newTestData(t)

	assert.True(t, d.Set("test", 1234))
	assert.False(t, d.Set("test", 1234))
	assert.True(t, d.Set("test", 4321))
}

func TestGet(t *testing.T) {
	d := newTestData(t)
	d.Set("test", 1234)

	data := d.Get()
	assert.Equal(t, 1234, data["test"])

	data["test"] = 4321
	assert.Equal(t, 1234, d.Get()["test"])
}

func TestDataGetPayloadOverlaysFieldsWithoutMutatingBase(t *testing.T) {
	d := newTestData(t)

	overlaid := d.GetPayload(map[string]interface{}{"overlay": "value", "flavor": "overridden"}, "first-id")
	assert.Equal(t, "first-id", overlaid.UUID)
	assert.Equal(t, "data-hostname", overlaid.Hostname)
	assert.Equal(t, "value", overlaid.Metadata["overlay"])
	assert.Equal(t, "overridden", overlaid.Metadata["flavor"])

	base := d.GetPayload(nil, "second-id")
	assert.Equal(t, "second-id", base.UUID)
	assert.NotContains(t, base.Metadata, "overlay")
	assert.Equal(t, flavor.GetFlavor(), base.Metadata["flavor"])
	assert.Equal(t, "value", overlaid.Metadata["overlay"])
}

func TestDataGetPayloadContainsFullAgentBaseFields(t *testing.T) {
	d := newTestData(t)

	payload := d.GetPayload(nil, "id")
	assert.Equal(t, version.AgentVersion, payload.Metadata["agent_version"])
	assert.Equal(t, version.AgentPackageVersion, payload.Metadata["package_version"])
	assert.Equal(t, d.conf.StartTime().UnixMilli(), payload.Metadata["agent_startup_time_ms"])
	assert.Equal(t, flavor.GetFlavor(), payload.Metadata["flavor"])
	assert.Equal(t, "full", payload.Metadata["infrastructure_mode"])
	for _, key := range []string{"install_method_tool", "install_method_tool_version", "install_method_installer_version"} {
		assert.Contains(t, payload.Metadata, key)
	}
}

func TestDataInitErrorInstallInfo(t *testing.T) {
	defer func() { installinfoGet = installinfo.Get }()
	installinfoGet = func(pkgconfigmodel.Reader) (*installinfo.InstallInfo, error) {
		return nil, errors.New("some error")
	}

	d := newTestData(t)
	assert.Equal(t, "undefined", d.data["install_method_tool"])
	assert.Equal(t, "", d.data["install_method_tool_version"])
	assert.Equal(t, "", d.data["install_method_installer_version"])

	installinfoGet = func(pkgconfigmodel.Reader) (*installinfo.InstallInfo, error) {
		return &installinfo.InstallInfo{
			Tool:             "test_tool",
			ToolVersion:      "1.2.3",
			InstallerVersion: "4.5.6",
		}, nil
	}

	d.initData()
	assert.Equal(t, "test_tool", d.data["install_method_tool"])
	assert.Equal(t, "1.2.3", d.data["install_method_tool_version"])
	assert.Equal(t, "4.5.6", d.data["install_method_installer_version"])
}

func TestDataGetConfigs(t *testing.T) {
	hostname, _ := hostnamemock.NewMock("data-hostname")
	d := NewData(config.NewMockWithOverrides(t, map[string]any{
		"inventories_configuration_enabled": true,
	}), logmock.New(t), hostname)

	data := make(map[string]interface{})
	d.getConfigs(data)

	keys := []string{}
	for k := range data {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	expected := []string{
		"provided_configuration",
		"full_configuration",
		"file_configuration",
		"environment_variable_configuration",
		"agent_runtime_configuration",
		"fleet_policies_configuration",
		"remote_configuration",
		"cli_configuration",
		"source_local_configuration",
	}
	sort.Strings(expected)

	assert.Equal(t, expected, keys)
}
