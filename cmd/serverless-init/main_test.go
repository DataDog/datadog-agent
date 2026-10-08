// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	serverlessInitInventory "github.com/DataDog/datadog-agent/cmd/serverless-init/inventory"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	serverlessInitTag "github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	agentmock "github.com/DataDog/datadog-agent/comp/logs/agent/mock"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	inventoryagentimpl "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/impl"
	runner "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	runnerfx "github.com/DataDog/datadog-agent/comp/metadata/runner/fx"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics/metricstest"
	serverlessTag "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type inventoryRecordingSerializer struct {
	serializer.MetricSerializer
	mu       sync.Mutex
	payloads [][]byte
}

func (s *inventoryRecordingSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	data, err := payload.MarshalJSON()
	if err == nil {
		s.mu.Lock()
		s.payloads = append(s.payloads, data)
		s.mu.Unlock()
	}
	return err
}

func (s *inventoryRecordingSerializer) Payloads() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.payloads)
}

type inventoryTestCloudService struct {
	cloudservice.CloudService
	data cloudservice.InventoryData
}

func (s inventoryTestCloudService) CanCollectInventory() bool { return s.data.ResourceID != "" }
func (s inventoryTestCloudService) GetInventoryData() cloudservice.InventoryData {
	return s.data
}

func TestInventoryOneShotReadiness(t *testing.T) {
	for _, firstRunDelay := range []int{0, 3600} {
		t.Run(fmt.Sprint(firstRunDelay), func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "")
			conf := coreconfig.NewMock(t)
			for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
				conf.Set(key, true, configmodel.SourceAgentRuntime)
			}
			conf.Set("inventories_first_run_delay", firstRunDelay, configmodel.SourceAgentRuntime)
			conf.Set("inventories_configuration_enabled", false, configmodel.SourceAgentRuntime)
			conf.Set("site", "datadoghq.eu", configmodel.SourceAgentRuntime)
			service := inventoryTestCloudService{data: cloudservice.InventoryData{
				ResourceID: "test-resource", ResourceName: "test-app", WorkloadType: "test-workload",
				ParentResourceID: "test-parent", Region: "test-region", RuntimeCandidates: []string{"python"},
			}}
			configureInventory(service)
			serial := &inventoryRecordingSerializer{}
			hostname, _ := hostnamemock.NewMock("inventory-test")
			logger := logmock.New(t)
			collected := make(chan struct{})
			var firstCollection sync.Once

			// Unlike TestOneShot (graph validation), OneShot actually starts Fx and
			// the real runner. Wait for its first provider callback to finish before
			// doing the same publication as setup. No scheduler timing is assumed.
			err := fxutil.OneShot(func(ia inventoryagent.Component, _ runner.Component) error {
				select {
				case <-collected:
				case <-time.After(5 * time.Second):
					return fmt.Errorf("runner did not collect before initialization")
				}
				ia.Submit()
				if len(serial.Payloads()) != 0 {
					return fmt.Errorf("inventory emitted before initialization")
				}
				serverlessInitInventory.UpdateAndSubmit(ia, service, mode.Conf{SidecarMode: true}, conf, map[string]string{
					"env": "test-env", "service": "test-service", "version": "test-version",
				})
				if len(serial.Payloads()) == 0 {
					return fmt.Errorf("publication did not synchronously enqueue inventory")
				}
				return nil
			},
				fx.Provide(serverlessInitInventory.NewCapabilities),
				fx.Provide(func(caps *inventoryagent.Capabilities) (inventoryagent.Component, runner.Provider) {
					p := inventoryagentimpl.NewComponent(inventoryagentimpl.Requires{
						Config: conf, Log: logger, Hostname: hostname, Serializer: serial, Capabilities: caps,
					})
					callback := p.Provider.Callback
					require.NotNil(t, callback, "readiness must not unregister an enabled provider")
					return p.Comp, runner.NewProvider(func(ctx context.Context) time.Duration {
						interval := callback(ctx)
						firstCollection.Do(func() { close(collected) })
						return interval
					})
				}),
				fx.Provide(func() coreconfig.Component { return conf }),
				fx.Provide(func() logdef.Component { return logger }),
				runnerfx.Module(),
				fx.StartTimeout(5*time.Second),
				fx.StopTimeout(5*time.Second),
			)
			require.NoError(t, err)
			payloads := serial.Payloads()
			require.NotEmpty(t, payloads)
			var startupUUID string
			for index, data := range payloads {
				var payload struct {
					UUID     string                 `json:"uuid"`
					Metadata map[string]interface{} `json:"agent_metadata"`
				}
				require.NoError(t, json.Unmarshal(data, &payload))
				require.NotEmpty(t, payload.UUID)
				if index == 0 {
					startupUUID = payload.UUID
					assert.Equal(t, "startup", payload.Metadata["report_reason"])
				}
				assert.Equal(t, startupUUID, payload.UUID)
				for key, expected := range map[string]string{
					"resource_id": "test-resource", "resource_name": "test-app", "workload_type": "test-workload",
					"parent_resource_id": "test-parent", "region": "test-region", "runtime": "python",
					"deployment_model": "sidecar", "flavor": "serverless-init",
					"dd_site": "datadoghq.eu", "dd_env": "test-env", "dd_service": "test-service", "dd_version": "test-version",
					"agent_version_base": version.AgentVersion, "serverless_init_version": serverlessTag.GetExtensionVersion(),
				} {
					assert.Equal(t, expected, payload.Metadata[key], key)
				}
				assert.Equal(t, float64(conf.StartTime().UnixMilli()), payload.Metadata["agent_startup_time_ms"])
			}
		})
	}
}

func TestPreloadEarlyPreservesInventoryFirstRunDelay(t *testing.T) {
	for _, delay := range []string{"", "3600"} {
		t.Run("delay="+delay, func(t *testing.T) {
			t.Setenv("DD_INVENTORIES_FIRST_RUN_DELAY", delay)
			if delay == "" {
				require.NoError(t, os.Unsetenv("DD_INVENTORIES_FIRST_RUN_DELAY"))
			}
			conf := configmock.New(t)
			before := conf.GetInt("inventories_first_run_delay")
			require.Positive(t, before)
			preloadEarly()
			assert.Equal(t, before, conf.GetInt("inventories_first_run_delay"))
		})
	}
}

func TestInventoryIdentityGate(t *testing.T) {
	originalConfig := pkgconfigsetup.Datadog()
	originalConfigPath := originalConfig.ConfigFileUsed()
	for _, scenario := range []string{"valid", "missing", "serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
		t.Run(scenario, func(t *testing.T) {
			conf := coreconfig.NewMock(t)
			require.NotSame(t, originalConfig, pkgconfigsetup.Datadog())
			configPath := filepath.Join(t.TempDir(), "datadog.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte("inventories_enabled: true\n"), 0600))
			pkgconfigsetup.Datadog().(configmodel.BuildableConfig).SetConfigFile(configPath)
			require.Equal(t, configPath, conf.ConfigFileUsed())
			for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
				conf.Set(key, true, configmodel.SourceAgentRuntime)
			}
			conf.Set("inventories_first_run_delay", 0, configmodel.SourceAgentRuntime)
			conf.Set("inventories_configuration_enabled", false, configmodel.SourceAgentRuntime)
			conf.Set("site", "datadoghq.eu", configmodel.SourceAgentRuntime)
			conf.Set("logs_enabled", true, configmodel.SourceAgentRuntime)
			conf.Set("config_id", "local-fleet-config", configmodel.SourceAgentRuntime)
			conf.Set("fleet_layers", []string{"local-fleet-policy"}, configmodel.SourceAgentRuntime)
			service := inventoryTestCloudService{data: cloudservice.InventoryData{
				ResourceID: "test-resource", ResourceName: "test-app", WorkloadType: "azure_app_service",
			}}
			if scenario == "missing" {
				service.data.ResourceID = ""
			} else if scenario != "valid" {
				conf.Set(scenario, false, configmodel.SourceAgentRuntime)
			}
			configureInventory(service)
			// setup reloads configuration after the pre-Fx gate.
			require.NoError(t, pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), secretsmock.New(t), delegatedauthmock.New(t), nil))
			serial := &inventoryRecordingSerializer{}
			hostname, _ := hostnamemock.NewMock("inventory-test")
			provides := inventoryagentimpl.NewComponent(inventoryagentimpl.Requires{
				Config: conf, Log: logmock.New(t), Hostname: hostname, Serializer: serial, Capabilities: serverlessInitInventory.NewCapabilities(),
			})
			serverlessInitInventory.UpdateAndSubmit(provides.Comp, service, mode.Conf{}, conf, map[string]string{
				"env": "test-env", "service": "test-service", "version": "test-version",
			})
			if scenario != "valid" {
				assert.Empty(t, serial.Payloads())
				assert.Nil(t, provides.Provider.Callback, "no periodic or in-flight collection may be registered")
				return
			}
			require.Len(t, serial.Payloads(), 1, "startup submission is synchronous")
			require.NotNil(t, provides.Provider.Callback)
			provides.Provider.Callback(context.Background())
			require.Len(t, serial.Payloads(), 2)
			var startupUUID string
			for index, data := range serial.Payloads() {
				var payload struct {
					UUID     string                 `json:"uuid"`
					Metadata map[string]interface{} `json:"agent_metadata"`
				}
				require.NoError(t, json.Unmarshal(data, &payload))
				assert.Equal(t, service.data.ResourceID, payload.Metadata["resource_id"])
				assert.Equal(t, service.data.ResourceName, payload.Metadata["resource_name"])
				assert.Equal(t, service.data.WorkloadType, payload.Metadata["workload_type"])
				assert.Equal(t, "datadoghq.eu", payload.Metadata["dd_site"], "site is explicitly injected, not collected by the generic refresh")
				assert.Equal(t, "test-env", payload.Metadata["dd_env"])
				assert.Equal(t, "test-service", payload.Metadata["dd_service"])
				assert.Equal(t, "test-version", payload.Metadata["dd_version"])
				assert.Equal(t, float64(conf.StartTime().UnixMilli()), payload.Metadata["agent_startup_time_ms"])
				assert.Contains(t, payload.Metadata, "install_method_tool")
				assert.Equal(t, "full", payload.Metadata["infrastructure_mode"])
				for _, field := range []string{
					"config_site", "feature_logs_enabled", "fleet_policies_applied", "config_id",
					"application_monitoring_config", "application_monitoring_config_fleet",
				} {
					assert.NotContains(t, payload.Metadata, field, "full-agent local refresh is intentionally skipped")
				}
				assert.NotContains(t, payload.Metadata, "full_configuration", "configuration payloads remain separately controlled")
				assert.Equal(t, "serverless-init", payload.Metadata["flavor"])
				assert.Equal(t, version.AgentVersion, payload.Metadata["agent_version_base"])
				assert.Equal(t, serverlessTag.GetExtensionVersion(), payload.Metadata["serverless_init_version"])
				require.NotEmpty(t, payload.UUID)
				if index == 0 {
					startupUUID = payload.UUID
				} else {
					assert.Equal(t, startupUUID, payload.UUID, "periodic inventory retains the process UUID")
				}
			}
		})
		// NewMock's cleanup restores the original config, including its path,
		// after the subtest's temporary configuration file has been removed.
		require.Same(t, originalConfig, pkgconfigsetup.Datadog())
		assert.Equal(t, originalConfigPath, pkgconfigsetup.Datadog().ConfigFileUsed())
	}
}

func TestInventorySerializesMissingValues(t *testing.T) {
	t.Setenv("DD_SERVERLESS_INIT_INVENTORY_WRAPPED_COMMAND_ENABLED", "")
	require.NoError(t, os.Unsetenv("DD_SERVERLESS_INIT_INVENTORY_WRAPPED_COMMAND_ENABLED"))
	originalCommit := version.Commit
	originalArgs := os.Args
	t.Cleanup(func() {
		version.Commit = originalCommit
		os.Args = originalArgs
	})
	conf := coreconfig.NewMock(t)
	for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
		conf.Set(key, true, configmodel.SourceAgentRuntime)
	}
	conf.Set("inventories_first_run_delay", 0, configmodel.SourceAgentRuntime)
	conf.Set("inventories_configuration_enabled", false, configmodel.SourceAgentRuntime)
	serial := &inventoryRecordingSerializer{}
	hostname, _ := hostnamemock.NewMock("inventory-test")
	provides := inventoryagentimpl.NewComponent(inventoryagentimpl.Requires{
		Config: conf, Log: logmock.New(t), Hostname: hostname, Serializer: serial, Capabilities: serverlessInitInventory.NewCapabilities(),
	})
	require.NotNil(t, provides.Provider.Callback)
	provides.Comp.Set("install_method_tool_version", "")

	populatedValues := map[string]string{
		"parent_resource_id": "test-parent", "region": "test-region", "gcp_project_id": "test-project",
		"aws_account_id": "123456789012", "azure_subscription_id": "test-subscription", "azure_resource_group": "test-group",
		"agent_commit": "abcdef1",
		"dd_env":       "test-env", "dd_service": "test-service", "dd_version": "test-version", "dd_site": "datadoghq.eu",
	}
	var previousTimestamp int64
	var commandPreviouslyPopulated bool
	for _, stage := range []struct {
		name                  string
		populated             bool
		sidecar               bool
		override              string
		metadata              []string
		command               []string
		wantRuntime           interface{}
		wrappedCommandEnabled string
		wantWrappedCommand    string
	}{
		{name: "initially missing", sidecar: true},
		{name: "populated", sidecar: true, populated: true, metadata: []string{"python"}, wantRuntime: "python"},
		{name: "cleared", sidecar: true},
		{name: "repopulated", sidecar: true, populated: true, metadata: []string{"python"}, wantRuntime: "python"},
		{name: "sidecar override wins", sidecar: true, override: " MyCustomRuntime ", metadata: []string{"Java", "Python"}, command: []string{"ruby"}, wantRuntime: "MyCustomRuntime"},
		{name: "sidecar ignores own command and clears override", sidecar: true, override: " UnKnOwN ", metadata: []string{"Container"}, command: []string{"ruby"}},
		{name: "sidecar metadata beats command", sidecar: true, metadata: []string{"Ruby"}, command: []string{"node"}, wantRuntime: "Ruby"},
		{name: "invalid worker falls back to stack", sidecar: true, metadata: []string{" UnKnOwN ", " Python "}, wantRuntime: "Python"},
		{name: "missing candidates clear runtime", sidecar: true, metadata: []string{"container", "null"}},
		{name: "sidecar ignores Python launcher", sidecar: true, command: []string{"ddtrace-run", "python", "app.py"}},
		{name: "sidecar ignores Ruby launcher", sidecar: true, command: []string{"bundle", "exec", "ruby", "app.rb"}},
		{name: "sidecar ignores Python entry point", sidecar: true, command: []string{"ddtrace-run", "gunicorn", "app:app"}},
		{name: "sidecar ignores Ruby entry point", sidecar: true, command: []string{"bundle", "exec", "puma"}},
		{name: "sidecar ignores bare Python entry point", sidecar: true, command: []string{"gunicorn", "app:app"}},
		{name: "sidecar ignores bare Ruby entry point", sidecar: true, command: []string{"puma"}},
		{name: "invalid override falls back to command", override: " NuLl ", command: []string{"/usr/bin/python3.12", "app.py", "--password=secret"}, wantRuntime: "Python"},
		{name: "Python launcher detection", command: []string{"ddtrace-run", "python", "app.py"}, wantRuntime: "Python"},
		{name: "Ruby launcher detection", command: []string{"bundle", "exec", "ruby", "app.rb"}, wantRuntime: "Ruby"},
		{name: "bare Python entry point detection", command: []string{"gunicorn", "app:app"}, wantRuntime: "Python"},
		{name: "Python entry point detection", command: []string{"ddtrace-run", "gunicorn", "app:app"}, wantRuntime: "Python"},
		{name: "unrecognized Python entry point clears runtime", command: []string{"ddtrace-run", "node", "app.py"}},
		{name: "bare Ruby entry point detection", command: []string{"puma"}, wantRuntime: "Ruby"},
		{name: "Ruby entry point detection", command: []string{"bundle", "exec", "puma"}, wantRuntime: "Ruby"},
		{name: "unrecognized Ruby entry point clears runtime", command: []string{"bundle", "exec", "node", "app.rb"}},
		{name: "Python launcher override wins", override: "MyPython", command: []string{"ddtrace-run", "gunicorn", "app:app"}, wantRuntime: "MyPython"},
		{name: "incomplete Python launcher clears runtime", command: []string{"ddtrace-run"}},
		{name: "Ruby launcher metadata wins", metadata: []string{"MyRuby"}, command: []string{"bundle", "exec", "puma"}, wantRuntime: "MyRuby"},
		{name: "incomplete Ruby launcher clears runtime", command: []string{"bundle", "exec"}},
		{name: "ambiguous command clears detection", metadata: []string{"unknown"}, command: []string{"sh", "-c", "node app.js"}},
		{name: "blank override and null metadata remain missing", override: " \t", metadata: []string{"null"}, command: []string{"./custom-app"}},
		{name: "command omitted by default", populated: true, command: []string{"python", "app.py", "--token=secret", "--password", "secret"}, wantRuntime: "Python"},
		{name: "command explicitly disabled", populated: true, wrappedCommandEnabled: "false", command: []string{"python", "app.py", "--token=secret", "--password", "secret"}, wantRuntime: "Python"},
		{name: "command opted in", populated: true, wrappedCommandEnabled: "true", command: []string{"python", "app.py", "--password=secret"}, wantRuntime: "Python", wantWrappedCommand: "python app.py --password=********"},
		{name: "command opt-out clears cache", populated: true, wrappedCommandEnabled: "false", command: []string{"python", "app.py", "--token=secret", "--password", "secret"}, wantRuntime: "Python"},
		{name: "command opted in again", wrappedCommandEnabled: "true", command: []string{"python", "app.py"}, wantRuntime: "Python", wantWrappedCommand: "python app.py"},
		{name: "sidecar clears command", sidecar: true, wrappedCommandEnabled: "true", command: []string{"python", "app.py"}},
		{name: "command repopulated", wrappedCommandEnabled: "true", command: []string{"python", "app.py"}, wantRuntime: "Python", wantWrappedCommand: "python app.py"},
		{name: "no command clears cache", wrappedCommandEnabled: "true"},
	} {
		t.Run(stage.name, func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", stage.override)
			if stage.wrappedCommandEnabled != "" {
				conf.Set("serverless.inventory_wrapped_command_enabled", stage.wrappedCommandEnabled == "true", configmodel.SourceAgentRuntime)
			}
			os.Args = append([]string{"serverless-init"}, stage.command...)
			service := inventoryTestCloudService{data: cloudservice.InventoryData{
				ResourceID: "test-resource", ResourceName: "test-app", WorkloadType: "azure_app_service", RuntimeCandidates: stage.metadata,
			}}
			var tags map[string]string
			version.Commit = ""
			conf.Set("site", "", configmodel.SourceAgentRuntime)
			if stage.populated {
				service.data.ParentResourceID = populatedValues["parent_resource_id"]
				service.data.Region = populatedValues["region"]
				service.data.GCPProjectID = populatedValues["gcp_project_id"]
				service.data.AWSAccountID = populatedValues["aws_account_id"]
				service.data.AzureSubscriptionID = populatedValues["azure_subscription_id"]
				service.data.AzureResourceGroup = populatedValues["azure_resource_group"]
				version.Commit = populatedValues["agent_commit"]
				conf.Set("site", populatedValues["dd_site"], configmodel.SourceAgentRuntime)
				tags = map[string]string{
					"env": populatedValues["dd_env"], "service": populatedValues["dd_service"], "version": populatedValues["dd_version"],
				}
			}
			for _, reason := range []string{"startup", "periodic"} {
				payloadCount := len(serial.Payloads())
				before := time.Now().UnixNano()
				if reason == "startup" {
					serverlessInitInventory.UpdateAndSubmit(provides.Comp, service, mode.Conf{SidecarMode: stage.sidecar}, conf, tags)
				} else {
					provides.Provider.Callback(context.Background())
				}
				after := time.Now().UnixNano()
				require.Len(t, serial.Payloads(), payloadCount+1)
				var payload struct {
					Timestamp int64                  `json:"timestamp"`
					Metadata  map[string]interface{} `json:"agent_metadata"`
				}
				require.NoError(t, json.Unmarshal(serial.Payloads()[payloadCount], &payload))
				assert.GreaterOrEqual(t, payload.Timestamp, before)
				assert.LessOrEqual(t, payload.Timestamp, after)
				assert.Greater(t, payload.Timestamp, previousTimestamp)
				previousTimestamp = payload.Timestamp
				for key, expected := range populatedValues {
					if stage.populated {
						assert.Equal(t, expected, payload.Metadata[key], key)
					} else {
						assert.Nil(t, payload.Metadata[key], "%s must be absent or JSON null", key)
					}
				}
				assert.Equal(t, service.data.ResourceID, payload.Metadata["resource_id"])
				assert.Equal(t, service.data.ResourceName, payload.Metadata["resource_name"])
				assert.Equal(t, service.data.WorkloadType, payload.Metadata["workload_type"])
				assert.Equal(t, reason, payload.Metadata["report_reason"])
				assert.Equal(t, stage.wantRuntime, payload.Metadata["runtime"], "missing runtime must be absent or JSON null, including after clearing")
				assert.NotContains(t, payload.Metadata, "runtime_candidates")
				if stage.sidecar {
					assert.Equal(t, "sidecar", payload.Metadata["deployment_model"])
				} else {
					assert.Equal(t, "in-container", payload.Metadata["deployment_model"])
				}
				if stage.wantWrappedCommand != "" {
					assert.Equal(t, stage.wantWrappedCommand, payload.Metadata["wrapped_command"])
					commandPreviouslyPopulated = true
				} else if commandPreviouslyPopulated {
					assert.Contains(t, payload.Metadata, "wrapped_command")
					assert.Nil(t, payload.Metadata["wrapped_command"], "cleared cached commands serialize as JSON null")
				} else {
					assert.NotContains(t, payload.Metadata, "wrapped_command", "never-collected commands must be omitted")
				}
				assert.NotContains(t, string(serial.Payloads()[payloadCount]), "secret")
				assert.Contains(t, payload.Metadata, "install_method_tool_version")
				assert.Equal(t, "", payload.Metadata["install_method_tool_version"], "core metadata is not normalized")
				assert.NotContains(t, payload.Metadata, "deployment_id")
				assert.NotContains(t, payload.Metadata, "tags")
			}
		})
	}
}

func TestInventoryGateLeavesOtherPlatformsUnchanged(t *testing.T) {
	conf := configmock.New(t)
	conf.Set("serverless.inventory_enabled", true, configmodel.SourceAgentRuntime)
	conf.Set("inventories_enabled", true, configmodel.SourceAgentRuntime)
	configureInventory(&cloudservice.LocalService{})
	configureInventory(&cloudservice.MicroVM{})
	assert.True(t, conf.GetBool("inventories_enabled"))
}

// TestMetricAgentNoOpWithoutDemux verifies that the methods called by the
// lifecycle server on the metric agent are safe when the agent has not been
// started (Demux is nil). In the no-API-key path the agent is never started,
// so /suspend and /terminate must not panic when they call Flush.
func TestMetricAgentNoOpWithoutDemux(t *testing.T) {
	agent := &metrics.ServerlessMetricAgent{}
	assert.NotPanics(t, func() {
		agent.Flush()
	})
}

func TestTagsSetup(t *testing.T) {
	configmock.New(t)

	modeConf = mode.DetectMode()

	t.Setenv("DD_TAGS", "key1:value1 key2:value2 key3:value3:4")
	t.Setenv("DD_EXTRA_TAGS", "key22:value22 key23:value23")

	t.Setenv("DD_SERVICE", "test-service")
	t.Setenv("DD_ENV", "test-env")
	t.Setenv("DD_VERSION", "1.0.0")

	cloudService := &cloudservice.LocalService{}
	tagConfig := configureTags(cloudService)

	baseTags := serverlessTag.MapToArray(serverlessInitTag.GetBaseTagsMap())
	cloudServiceTags := cloudService.GetTags()
	enhancedFromCloudService := cloudService.GetEnhancedMetricTags(cloudServiceTags)
	cloudServiceEnhancedMetricTags := enhancedFromCloudService.Base
	cloudServiceEnhancedUsageMetricTags := enhancedFromCloudService.Usage

	versionTag := "_dd.datadog_init_version:xxx"
	enhancedMetricVersionTags := []string{"datadog_init_version:xxx", "sidecar:false"}

	assert.ElementsMatch(t, slices.Concat(tagConfig.ConfiguredTags, baseTags, serverlessTag.MapToArray(cloudServiceTags), []string{versionTag}), serverlessTag.MapToArray(tagConfig.Tags))
	assert.ElementsMatch(t, slices.Concat(tagConfig.ConfiguredTags, baseTags, serverlessTag.MapToArray(cloudServiceEnhancedMetricTags), enhancedMetricVersionTags), serverlessTag.MapToArray(tagConfig.EnhancedMetricTags))
	assert.ElementsMatch(t, slices.Concat(serverlessTag.MapToArray(cloudServiceEnhancedUsageMetricTags), enhancedMetricVersionTags), serverlessTag.MapToArray(tagConfig.EnhancedUsageMetricTags))
}

func TestFxApp(t *testing.T) {
	fxutil.TestOneShot(t, main)
}

func TestFlushLogsAgentSuccess(t *testing.T) {
	mockLogsAgent := agentmock.NewMockServerlessLogsAgent()
	flushLogsAgent(100*time.Millisecond, mockLogsAgent)
	assert.Equal(t, true, mockLogsAgent.DidFlush())
}

func TestFlushLogsAgentTimeout(t *testing.T) {
	mockLogsAgent := agentmock.NewMockServerlessLogsAgent()
	mockLogsAgent.SetFlushDelay(time.Hour)

	flushLogsAgent(100*time.Millisecond, mockLogsAgent)
	assert.Equal(t, false, mockLogsAgent.DidFlush())
}

func TestFlushLogsAgentNil(t *testing.T) {
	assert.NotPanics(t, func() {
		flushLogsAgent(100*time.Millisecond, nil)
	})
}

// TestSetupWithoutAPIKey verifies that when DD_API_KEY is not set, the metric
// agent is left as a bare struct (Demux nil) and reporting metrics through it
// is a safe no-op. sendMetricSample (in pkg/serverless/metrics/metric.go) takes
// the early-return path when Demux is nil, preventing noisy panics when
// serverless-init is used without configuration.
func TestSetupWithoutAPIKey(t *testing.T) {
	metricAgent := &metrics.ServerlessMetricAgent{}
	assert.NotPanics(t, func() {
		metricAgent.AddEnhancedMetric("enhanced.metric", 1.0, pkgmetrics.MetricSourceServerless, 0)
		metricAgent.AddLegacyEnhancedMetric("legacy.metric", 1.0, pkgmetrics.MetricSourceServerless)
	})
}

// TestLogTagsBaseComputedFromTagConfigTags verifies that the logTagsBase variable
// computed in setup() — as serverlessTag.MapToArray(tagConfig.Tags) — contains all
// tags configured via DD_TAGS. This documents the contract: BaseTags passed to
// LifecycleContext must be the full startup tag slice so the lifecycle server can
// append microvm_id to it without losing any base tags.
func TestLogTagsBaseComputedFromTagConfigTags(t *testing.T) {
	configmock.New(t)
	modeConf = mode.DetectMode()
	t.Setenv("DD_TAGS", "env:prod region:us-east-1")

	cloudService := &cloudservice.LocalService{}
	tagConfig := configureTags(cloudService)
	logTagsBase := serverlessTag.MapToArray(tagConfig.Tags)

	assert.Contains(t, logTagsBase, "env:prod",
		"logTagsBase must include all DD_TAGS entries (used as BaseTags on the lifecycle server)")
	assert.Contains(t, logTagsBase, "region:us-east-1")
	assert.NotEmpty(t, logTagsBase)
}

// TestBaseTraceTagsComputedFromTagConfigTags verifies that traceTags —
// passed as BaseTraceTags to LifecycleContext — contains all tags from DD_TAGS
// so that the lifecycle server can extend the map with lambda_microvm_id at /run
// without losing any startup tags.
func TestBaseTraceTagsComputedFromTagConfigTags(t *testing.T) {
	configmock.New(t)
	modeConf = mode.DetectMode()
	t.Setenv("DD_TAGS", "env:prod region:us-east-1")

	cloudService := &cloudservice.LocalService{}
	tagConfig := configureTags(cloudService)
	baseTraceTags := serverlessInitTag.MakeTraceAgentTags(tagConfig.Tags)

	assert.Equal(t, "prod", baseTraceTags["env"],
		"BaseTraceTags must include all DD_TAGS entries (used as BaseTraceTags on the lifecycle server)")
	assert.Equal(t, "us-east-1", baseTraceTags["region"])
	assert.NotEmpty(t, baseTraceTags)
}

// TestSetupOtlpAgentNoPanic ensures setupOtlpAgent does not panic when OTLP is enabled.
func TestSetupOtlpAgentNoPanic(t *testing.T) {
	t.Setenv("DD_OTLP_CONFIG_LOGS_ENABLED", "true")
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:4317")

	configmock.New(t)
	_ = pkgconfigsetup.LoadDatadog(pkgconfigsetup.Datadog(), secretsmock.New(t), delegatedauthmock.New(t), nil)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	bundle := metricstest.New(t, fakeTagger)
	metricAgent := metrics.New(bundle.Demux, metrics.Tags{})

	assert.NotPanics(t, func() { setupOtlpAgent(metricAgent, fakeTagger) })

	// Timeout to allow the goroutine in ServerlessOTLPAgent.Start() to run.
	// If it panics the process crashes. Without this the test can pass flakily when the goroutine hasn't run yet.
	const panicWindow = 500 * time.Millisecond
	<-time.After(panicWindow)
}

// TestRun_LocalService_InitMode executes the user app through LocalService.Run
// in init-container mode, verifying the CloudService.Run interface routes
// correctly to RunInit(cfg, nil) — no child tracking, user app runs normally.
func TestRun_LocalService_InitMode(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init", "sh", "-c", "exit 0"}

	svc := &cloudservice.LocalService{}
	err := svc.Run(mode.Conf{SidecarMode: false}, &serverlessInitLog.Config{})
	assert.NoError(t, err)
}

// TestRun_LocalService_SidecarMode verifies that the defaultRun sidecar path
// calls RunSidecar (not RunInit). RunSidecar registers a real signal.Notify
// for SIGINT/SIGTERM and blocks until one arrives, in both production and
// tests, so we send ourselves a real SIGTERM to let it return instead of
// leaking a goroutine that intercepts SIGTERM for the rest of the test
// binary's life — which would otherwise swallow a genuine SIGTERM (e.g. CI
// cancellation) instead of letting the process terminate.
func TestRun_LocalService_SidecarMode(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()
	os.Args = []string{"datadog-init"} // sidecar mode: no cmd args

	// Register our own listener first so the default terminate-on-SIGTERM
	// disposition is already overridden before we signal ourselves below,
	// regardless of whether RunSidecar's own signal.Notify has run yet.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGTERM)
	defer signal.Stop(guard)

	svc := &cloudservice.LocalService{}
	done := make(chan error, 1)
	assert.NotPanics(t, func() {
		go func() { done <- svc.Run(mode.Conf{SidecarMode: true}, &serverlessInitLog.Config{}) }()
	})

	// Give RunSidecar's own signal.Notify time to register before we signal.
	time.Sleep(50 * time.Millisecond)
	assert.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("RunSidecar did not return after SIGTERM")
	}
	<-guard // drain our own copy so it doesn't leak into later tests
}
