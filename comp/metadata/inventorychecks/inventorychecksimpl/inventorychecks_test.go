// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorychecksimpl

import (
	"expvar"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	logsBundle "github.com/DataDog/datadog-agent/comp/logs"
	logagent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func getTestInventoryChecks(t *testing.T, coll optional.Option[collector.Component], logAgent optional.Option[logagent.Component], overrides map[string]any) *inventorychecksImpl {
	p := newInventoryChecksProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: overrides}),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			fx.Provide(func() optional.Option[collector.Component] {
				return coll
			}),
			fx.Provide(func() optional.Option[logagent.Component] {
				return logAgent
			}),
		),
	)
	return p.Comp.(*inventorychecksImpl)
}

func TestSet(t *testing.T) {
	ic := getTestInventoryChecks(
		t, optional.NewNoneOption[collector.Component](), optional.Option[logagent.Component]{}, nil,
	)

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
	ic := getTestInventoryChecks(
		t, optional.NewNoneOption[collector.Component](), optional.Option[logagent.Component]{}, nil,
	)

	ic.Set("", "key", "value")

	assert.Len(t, ic.data, 0)
}

func TestGetInstanceMetadata(t *testing.T) {
	ic := getTestInventoryChecks(
		t, optional.NewNoneOption[collector.Component](), optional.Option[logagent.Component]{}, nil,
	)

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
	for _, invChecksCfgEnabled := range []bool{true, false} {
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

		mockColl := fxutil.Test[collector.Component](t,
			fx.Replace(collectorimpl.MockParams{
				ChecksInfo: cInfo,
			}),
			collectorimpl.MockModule(),
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		)

		// Setup log sources
		logSources := sources.NewLogSources()
		src := sources.NewLogSource("redisdb", &logConfig.LogsConfig{
			Type:       logConfig.FileType,
			Path:       "/var/log/redis/redis.log",
			Identifier: "redisdb",
			Service:    "awesome_cache",
			Source:     "redis",
			Tags:       []string{"env:prod"},
		})
		// Register an error
		src.Status.Error(fmt.Errorf("No such file or directory"))
		logSources.AddSource(src)
		fakeTagger := taggerimpl.SetupFakeTagger(t)

		mockLogAgent := fxutil.Test[optional.Option[logagent.Mock]](
			t,
			logsBundle.MockBundle(),
			core.MockBundle(),
			inventoryagentimpl.MockModule(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			fx.Provide(func() tagger.Component {
				return fakeTagger
			}),
		)
		logsAgent, _ := mockLogAgent.Get()
		logsAgent.SetSources(logSources)

		testName := fmt.Sprintf("inventories_checks_configuration_enabled=%t", invChecksCfgEnabled)
		t.Run(testName, func(t *testing.T) {
			overrides := map[string]any{
				"inventories_configuration_enabled":        true,
				"inventories_checks_configuration_enabled": invChecksCfgEnabled,
			}

			ic := getTestInventoryChecks(t,
				optional.NewOption[collector.Component](mockColl),
				optional.NewOption[logagent.Component](logsAgent),
				overrides,
			)

			ic.hostname = "test-hostname"

			ic.Set("check1_instance1", "check_provided_key1", 123)
			ic.Set("check1_instance1", "check_provided_key2", "Hi")
			ic.Set("non_running_checkid", "check_provided_key1", "this_should_not_be_kept")

			p := ic.getPayloadWithConfigs().(*Payload)

			assert.Equal(t, "test-hostname", p.Hostname)

			assert.Len(t, p.Metadata, 2)           // 'non_running_checkid' should have been cleaned
			assert.Len(t, p.Metadata["check1"], 2) // check1 has two instances

			check1Instance1 := p.Metadata["check1"][0]
			assert.Equal(t, "check1_instance1", check1Instance1["config.hash"])
			assert.Equal(t, "provider1", check1Instance1["config.provider"])
			assert.Equal(t, 123, check1Instance1["check_provided_key1"])
			assert.Equal(t, "Hi", check1Instance1["check_provided_key2"])
			if invChecksCfgEnabled {
				assert.Equal(t, "", check1Instance1["init_config"])
				assert.Equal(t, "test: 21", check1Instance1["instance_config"])
			} else {
				assert.Nil(t, check1Instance1["init_config"])
				assert.Nil(t, check1Instance1["instance_config"])
			}

			check1Instance2 := p.Metadata["check1"][1]
			assert.Equal(t, "check1_instance2", check1Instance2["config.hash"])
			assert.Equal(t, "provider1", check1Instance2["config.provider"])
			if invChecksCfgEnabled {
				assert.Equal(t, "", check1Instance2["init_config"])
				assert.Equal(t, "test: 22", check1Instance2["instance_config"])
			} else {
				assert.Nil(t, check1Instance2["init_config"])
				assert.Nil(t, check1Instance2["instance_config"])
			}

			assert.Len(t, p.Metadata["check2"], 1) // check2 has one instance
			check2Instance1 := p.Metadata["check2"][0]
			assert.Equal(t, "check2_instance1", check2Instance1["config.hash"])
			assert.Equal(t, "provider2", check2Instance1["config.provider"])
			if invChecksCfgEnabled {
				assert.Equal(t, "{}", check2Instance1["init_config"])
				assert.Equal(t, "{}", check2Instance1["instance_config"])
			} else {
				assert.Nil(t, check2Instance1["init_config"])
				assert.Nil(t, check2Instance1["instance_config"])
			}

			// Check that metadata linked to non-existing check were deleted
			assert.NotContains(t, "non_running_checkid", ic.data)

			// Check the log sources part of the metadata
			if invChecksCfgEnabled {
				assert.Len(t, p.LogsMetadata, 1)
				actualSource, found := p.LogsMetadata["redisdb"]
				assert.True(t, found)
				assert.Len(t, actualSource, 1)
				expectedSourceConfig := `{"type":"file","path":"/var/log/redis/redis.log","service":"awesome_cache","source":"redis","tags":["env:prod"]}`
				assert.Equal(t, expectedSourceConfig, actualSource[0]["config"])
				expectedSourceStatus := map[string]string{
					"status": "error",
					"error":  "Error: No such file or directory",
				}
				assert.Equal(t, expectedSourceStatus, actualSource[0]["state"])
				assert.Equal(t, "awesome_cache", actualSource[0]["service"])
				assert.Equal(t, "redis", actualSource[0]["source"])
				assert.Equal(t, []string{"env:prod"}, actualSource[0]["tags"])
			} else {
				assert.Len(t, p.LogsMetadata, 0)
			}
		})
	}
}

func TestFlareProviderFilename(t *testing.T) {
	ic := getTestInventoryChecks(
		t, optional.NewNoneOption[collector.Component](), optional.Option[logagent.Component]{}, nil,
	)
	assert.Equal(t, "checks.json", ic.FlareFileName)
}

// TODO (Component): This test will be removed when the inventorychecks component will be move into the collector component
func TestExpvarExist(t *testing.T) {
	getTestInventoryChecks(
		t, optional.NewNoneOption[collector.Component](), optional.Option[logagent.Component]{}, nil,
	)
	assert.NotNil(t, expvar.Get("inventories"))
}
