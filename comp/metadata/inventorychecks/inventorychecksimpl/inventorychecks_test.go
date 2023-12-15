// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorychecksimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func getTestInventoryChecks(t *testing.T, coll optional.Option[collector.Collector], overrides map[string]any) *inventorychecksImpl {
	p := newInventoryChecksProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: overrides}),
			fx.Provide(func() serializer.MetricSerializer { return &serializer.MockSerializer{} }),
			fx.Provide(func() optional.Option[collector.Collector] {
				return coll
			}),
		),
	)
	return p.Comp.(*inventorychecksImpl)
}

func TestSet(t *testing.T) {
	ic := getTestInventoryChecks(t, optional.NewNoneOption[collector.Collector](), nil)

	ic.Set("instance_1", "key", "value")

	assert.Len(t, ic.data, 1)
	assert.Contains(t, ic.data, "instance_1")
	assert.Contains(t, ic.data["instance_1"].metadata, "key")
	assert.Equal(t, ic.data["instance_1"].metadata["key"], "value")

	ic.Set("instance_1", "key2", "value2")

	assert.Len(t, ic.data, 1)
	assert.Contains(t, ic.data["instance_1"].metadata, "key2")
	assert.Equal(t, ic.data["instance_1"].metadata["key2"], "value2")
}

func TestSetEmptyInstance(t *testing.T) {
	ic := getTestInventoryChecks(t, optional.NewNoneOption[collector.Collector](), nil)

	ic.Set("", "key", "value")

	assert.Len(t, ic.data, 0)
}

func TestGetInstanceMetadata(t *testing.T) {
	ic := getTestInventoryChecks(t, optional.NewNoneOption[collector.Collector](), nil)

	ic.Set("instance_1", "key1", "value1")
	ic.Set("instance_1", "key2", "value2")

	res := ic.GetInstanceMetadata("instance_1")
	assert.Equal(t,
		map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
		res,
	)

	assert.Empty(t, ic.GetInstanceMetadata("instance_2"))
}

func TestGetPayload(t *testing.T) {
	cInfo := []check.Info{
		check.MockInfo{
			Name:         "check1",
			CheckID:      checkid.ID("check1_instance1"),
			Source:       "provider1",
			InitConf:     "",
			InstanceConf: "{\"test\":21}",
		},
		check.MockInfo{
			Name:         "check1",
			CheckID:      checkid.ID("check1_instance2"),
			Source:       "provider1",
			InitConf:     "",
			InstanceConf: "{\"test\":22}",
		},
		check.MockInfo{
			Name:         "check2",
			CheckID:      checkid.ID("check2_instance1"),
			Source:       "provider2",
			InitConf:     "{}",
			InstanceConf: "{}",
		},
	}

	overrides := map[string]any{
		"inventories_configuration_enabled":        true,
		"inventories_checks_configuration_enabled": true,
	}

	mockColl := collector.NewMock(cInfo)
	mockColl.On("AddEventReceiver", mock.AnythingOfType("EventReceiver")).Return()
	mockColl.On("MapOverChecks", mock.AnythingOfType("func([]check.Info)")).Return()

	ic := getTestInventoryChecks(t,
		optional.NewOption[collector.Collector](mockColl),
		overrides,
	)

	ic.hostname = "test-hostname"

	ic.Set("check1_instance1", "check_provided_key1", 123)
	ic.Set("check1_instance1", "check_provided_key2", "Hi")
	ic.Set("non_running_checkid", "check_provided_key1", "this_should_not_be_kept")

	p := ic.getPayload().(*Payload)

	assert.Equal(t, "test-hostname", p.Hostname)

	assert.Len(t, p.Metadata, 2)           // 'non_running_checkid' should have been cleaned
	assert.Len(t, p.Metadata["check1"], 2) // check1 has two instances

	check1Instance1 := p.Metadata["check1"][0]
	assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
	assert.Equal(t, "provider1", check1Instance1["config.provider"])
	assert.Equal(t, 123, check1Instance1["check_provided_key1"])
	assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
	assert.Equal(t, "", check1Instance1["init_config"])
	assert.Equal(t, "test: 21", check1Instance1["instance_config"])

	check1Instance2 := p.Metadata["check1"][1]
	assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
	assert.Equal(t, "provider1", check1Instance2["config.provider"])
	assert.Equal(t, "", check1Instance2["init_config"])
	assert.Equal(t, "test: 22", check1Instance2["instance_config"])

	assert.Len(t, p.Metadata["check2"], 1) // check2 has one instance
	check2Instance1 := p.Metadata["check2"][0]
	assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
	assert.Equal(t, "provider2", check2Instance1["config.provider"])
	assert.Equal(t, "{}", check2Instance1["init_config"])
	assert.Equal(t, "{}", check2Instance1["instance_config"])

	// Check that metadata linked to non-existing check were deleted
	assert.NotContains(t, "non_running_checkid", ic.data)
}

func TestFlareProviderFilename(t *testing.T) {
	ic := getTestInventoryChecks(t, optional.NewNoneOption[collector.Collector](), nil)
	assert.Equal(t, "checks.json", ic.FlareFileName)
}
