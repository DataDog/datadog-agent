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
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	serverlessInitInventory "github.com/DataDog/datadog-agent/cmd/serverless-init/inventory"
	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	serverlessInitTag "github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	agentmock "github.com/DataDog/datadog-agent/comp/logs/agent/mock"
	inventoryagentimpl "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/impl"
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
	payloads [][]byte
}

func (s *inventoryRecordingSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	data, err := payload.MarshalJSON()
	if err == nil {
		s.payloads = append(s.payloads, data)
	}
	return err
}

type inventoryTestCloudService struct {
	cloudservice.CloudService
	data cloudservice.InventoryData
}

func (s inventoryTestCloudService) CanCollectInventory() bool { return s.data.ResourceID != "" }
func (s inventoryTestCloudService) GetInventoryData() cloudservice.InventoryData {
	return s.data
}

func TestInventoryIdentityGate(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
		t.Run(scenario, func(t *testing.T) {
			conf := coreconfig.NewMock(t)
			configPath := filepath.Join(t.TempDir(), "datadog.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte("inventories_enabled: true\n"), 0600))
			pkgconfigsetup.Datadog().(configmodel.BuildableConfig).SetConfigFile(configPath)
			for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
				conf.Set(key, true, configmodel.SourceAgentRuntime)
			}
			conf.Set("inventories_first_run_delay", 0, configmodel.SourceAgentRuntime)
			conf.Set("inventories_configuration_enabled", false, configmodel.SourceAgentRuntime)
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
			serverlessInitInventory.Inject(provides.Comp, service, mode.Conf{}, conf, nil)
			serverlessInitInventory.Submit(provides.Comp, conf)
			if scenario != "valid" {
				assert.Empty(t, serial.payloads)
				assert.Nil(t, provides.Provider.Callback, "no periodic or in-flight collection may be registered")
				return
			}
			require.Len(t, serial.payloads, 1, "startup submission is synchronous")
			require.NotNil(t, provides.Provider.Callback)
			provides.Provider.Callback(context.Background())
			require.Len(t, serial.payloads, 2)
			for _, data := range serial.payloads {
				var payload struct {
					Metadata map[string]interface{} `json:"agent_metadata"`
				}
				require.NoError(t, json.Unmarshal(data, &payload))
				assert.Equal(t, service.data.ResourceID, payload.Metadata["resource_id"])
			}
		})
	}
}

// inventoryMetadataTransport supplies the real GCP builders with deterministic
// metadata without contacting the cloud metadata service or any other host.
type inventoryMetadataTransport map[string]string

func (m inventoryMetadataTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	value, ok := m[req.URL.Path]
	if req.URL.Host != "metadata.google.internal" || !ok {
		return nil, fmt.Errorf("unexpected inventory metadata request: %s", req.URL)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(value)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestInventorySerializesPlatformIdentity(t *testing.T) {
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = inventoryMetadataTransport{
		"/computeMetadata/v1/project/project-id": "test-project",
		"/computeMetadata/v1/instance/region":    "projects/123/regions/us-central1",
		"/computeMetadata/v1/instance/id":        "test-instance",
	}

	const gcpPrefix = "//run.googleapis.com/projects/test-project/locations/us-central1"
	const acaParent = "/subscriptions/test-subscription/resourcegroups/test-group/providers/microsoft.app/containerapps/test-app"
	for _, platform := range []struct {
		name               string
		env                map[string]string
		missingRequiredEnv string
		resourceID         string
		parentID           string
		resourceName       string
	}{
		{
			name: "cloud_run_service",
			env: map[string]string{
				"K_SERVICE": "test-service", "K_REVISION": "test-revision",
			},
			missingRequiredEnv: "K_SERVICE",
			resourceID:         gcpPrefix + "/revisions/test-revision",
			parentID:           gcpPrefix + "/services/test-service",
			resourceName:       "test-service",
		},
		{
			name: "cloud_run_function",
			env: map[string]string{
				"K_SERVICE": "test-function", "K_REVISION": "test-function-revision", "FUNCTION_TARGET": "handler",
			},
			missingRequiredEnv: "K_REVISION",
			resourceID:         gcpPrefix + "/revisions/test-function-revision",
			parentID:           gcpPrefix + "/services/test-function",
			resourceName:       "test-function",
		},
		{
			name: "cloud_run_job",
			env: map[string]string{
				"CLOUD_RUN_JOB": "test-job", "CLOUD_RUN_EXECUTION": "test-execution",
			},
			missingRequiredEnv: "CLOUD_RUN_JOB",
			resourceID:         gcpPrefix + "/executions/test-execution",
			parentID:           gcpPrefix + "/jobs/test-job",
			resourceName:       "test-job",
		},
		{
			name: "azure_container_app",
			env: map[string]string{
				"DD_AZURE_SUBSCRIPTION_ID": "Test-Subscription", "DD_AZURE_RESOURCE_GROUP": "Test-Group",
				"CONTAINER_APP_NAME": "Test-App", "CONTAINER_APP_REVISION": "Test-Revision",
			},
			missingRequiredEnv: "CONTAINER_APP_REVISION",
			resourceID:         acaParent + "/revisions/Test-Revision",
			parentID:           acaParent,
			resourceName:       "Test-App",
		},
		{
			name: "azure_app_service",
			env: map[string]string{
				"WEBSITE_OWNER_NAME": "Test-Subscription+webspace", "WEBSITE_RESOURCE_GROUP": "Test-Group",
				"WEBSITE_SITE_NAME": "Test-Site", "WEBSITE_STACK": "NODE",
			},
			missingRequiredEnv: "WEBSITE_SITE_NAME",
			resourceID:         "/subscriptions/test-subscription/resourcegroups/test-group/providers/microsoft.web/sites/test-site",
			resourceName:       "Test-Site",
		},
	} {
		t.Run(platform.name, func(t *testing.T) {
			for _, missing := range []bool{false, true} {
				t.Run(fmt.Sprintf("missing_required=%t", missing), func(t *testing.T) {
					for _, key := range []string{
						"AWS_LAMBDA_MICROVM_IMAGE_ARN", "K_SERVICE", "FUNCTION_TARGET", "CLOUD_RUN_JOB",
						"CONTAINER_APP_NAME", "WEBSITE_STACK", "FUNCTIONS_WORKER_RUNTIME",
					} {
						t.Setenv(key, "")
						require.NoError(t, os.Unsetenv(key))
					}
					for key, value := range platform.env {
						t.Setenv(key, value)
					}
					service := cloudservice.GetCloudServiceType()
					// An already-identified Gen2 function needs no function target for inventory.
					require.NoError(t, os.Unsetenv("FUNCTION_TARGET"))
					if missing {
						t.Setenv(platform.missingRequiredEnv, "")
					}
					require.Equal(t, !missing, service.CanCollectInventory())

					conf := coreconfig.NewMock(t)
					for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection"} {
						conf.Set(key, true, configmodel.SourceAgentRuntime)
					}
					conf.Set("inventories_first_run_delay", 0, configmodel.SourceAgentRuntime)
					conf.Set("inventories_configuration_enabled", false, configmodel.SourceAgentRuntime)
					configureInventory(service)
					serial := &inventoryRecordingSerializer{}
					hostname, _ := hostnamemock.NewMock("inventory-test")
					provides := inventoryagentimpl.NewComponent(inventoryagentimpl.Requires{
						Config: conf, Log: logmock.New(t), Hostname: hostname, Serializer: serial, Capabilities: serverlessInitInventory.NewCapabilities(),
					})
					serverlessInitInventory.Inject(provides.Comp, service, mode.Conf{SidecarMode: true}, conf, map[string]string{"service": "custom-dd-service"})
					serverlessInitInventory.Submit(provides.Comp, conf)
					if missing {
						assert.Empty(t, serial.payloads)
						assert.Nil(t, provides.Provider.Callback, "missing identity must suppress the periodic provider too")
						return
					}
					require.Len(t, serial.payloads, 1)
					require.NotNil(t, provides.Provider.Callback)
					provides.Provider.Callback(context.Background())
					require.Len(t, serial.payloads, 2)
					for _, data := range serial.payloads {
						var payload struct {
							Metadata map[string]interface{} `json:"agent_metadata"`
						}
						require.NoError(t, json.Unmarshal(data, &payload))
						assert.Equal(t, platform.resourceID, payload.Metadata["resource_id"])
						assert.Equal(t, platform.resourceName, payload.Metadata["resource_name"])
						assert.Equal(t, platform.name, payload.Metadata["workload_type"])
						if platform.parentID == "" {
							assert.Nil(t, payload.Metadata["parent_resource_id"], "no parent must be absent or JSON null")
						} else {
							assert.Equal(t, platform.parentID, payload.Metadata["parent_resource_id"])
						}
						assert.Equal(t, "custom-dd-service", payload.Metadata["dd_service"])
						assert.NotContains(t, payload.Metadata, "deployment_id")
					}
				})
			}
		})
	}
}

func TestInventorySerializesMissingValues(t *testing.T) {
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
	for _, stage := range []struct {
		name        string
		populated   bool
		sidecar     bool
		override    string
		metadata    []string
		command     []string
		wantRuntime interface{}
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
		{name: "invalid override falls back to command", override: " NuLl ", command: []string{"/usr/bin/python3.12", "app.py", "--password=secret"}, wantRuntime: "Python"},
		{name: "ambiguous command clears detection", metadata: []string{"unknown"}, command: []string{"sh", "-c", "node app.js"}},
		{name: "blank override and null metadata remain missing", override: " \t", metadata: []string{"null"}, command: []string{"./custom-app"}},
	} {
		t.Run(stage.name, func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", stage.override)
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
			serverlessInitInventory.Inject(provides.Comp, service, mode.Conf{SidecarMode: stage.sidecar}, conf, tags)

			for _, reason := range []string{"startup", "periodic"} {
				payloadCount := len(serial.payloads)
				before := time.Now().UnixNano()
				if reason == "startup" {
					serverlessInitInventory.Submit(provides.Comp, conf)
				} else {
					provides.Provider.Callback(context.Background())
				}
				after := time.Now().UnixNano()
				require.Len(t, serial.payloads, payloadCount+1)
				var payload struct {
					Timestamp int64                  `json:"timestamp"`
					Metadata  map[string]interface{} `json:"agent_metadata"`
				}
				require.NoError(t, json.Unmarshal(serial.payloads[payloadCount], &payload))
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
					assert.NotContains(t, payload.Metadata, "wrapped_command")
				} else {
					assert.Equal(t, "in-container", payload.Metadata["deployment_model"])
					assert.Contains(t, payload.Metadata["wrapped_command"], stage.command[0])
					assert.NotContains(t, payload.Metadata["wrapped_command"], "secret")
				}
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
