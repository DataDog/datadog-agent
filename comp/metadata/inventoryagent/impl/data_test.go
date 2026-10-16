// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func newTestData(t *testing.T) *Data {
	hostname, _ := hostnamemock.NewMock("data-hostname")
	return NewData(config.NewMock(t), logmock.New(t), hostname)
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
