// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	iainterface "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	configFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher"
	sysprobeConfigFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher/sysprobe"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/fips"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	sysprobecfg "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// testDeps is a bridge struct with fx.In so that fxutil.Test can inject dependencies.
// Requires uses no fx.In embed, so we use testDeps + makeRequires as an adapter.
type testDeps struct {
	fx.In

	Log            log.Component
	Config         config.Component
	SysProbeConfig option.Option[sysprobeconfig.Component]
	Serializer     serializer.MetricSerializer
	IPCClient      ipc.HTTPClient
	Hostname       hostnameinterface.Component
	Secrets        secrets.Component         `optional:"true"`
	Capabilities   *iainterface.Capabilities `optional:"true"`
}

func makeRequires(deps testDeps) Requires {
	return Requires{
		Log:            deps.Log,
		Config:         deps.Config,
		SysProbeConfig: deps.SysProbeConfig,
		Serializer:     deps.Serializer,
		IPCClient:      deps.IPCClient,
		Hostname:       deps.Hostname,
		Secrets:        deps.Secrets,
		Capabilities:   deps.Capabilities,
	}
}

func getProvides(t *testing.T, confOverrides map[string]any, sysprobeConfOverrides map[string]any, options ...fx.Option) Provides {
	sysprobeConf := sysprobeconfigmock.NewMockWithOverrides(t, sysprobeConfOverrides)
	return NewComponent(
		makeRequires(fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, confOverrides) }),
			fx.Provide(func() sysprobeconfig.Component { return sysprobeConf }),
			fxutil.ProvideOptional[sysprobeconfig.Component](),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
			hostnameimpl.MockModule(),
			fx.Options(options...),
		)),
	)
}

func getTestInventoryPayload(t *testing.T, confOverrides map[string]any, sysprobeConfOverrides map[string]any) *inventoryagent {
	p := getProvides(t, confOverrides, sysprobeConfOverrides)
	return p.Comp.(*inventoryagent)
}

func TestSet(t *testing.T) {
	ia := getTestInventoryPayload(t, nil, nil)

	ia.Set("test", 1234)
	assert.Equal(t, 1234, ia.data["test"])
}

func TestGetPayload(t *testing.T) {
	ia := getTestInventoryPayload(t, nil, nil)
	ia.hostname = "hostname-for-test"

	ia.Set("test", 1234)
	startTime := time.Now().UnixNano()

	p := ia.getPayload()
	payload := p.(*Payload)

	assert.True(t, payload.Timestamp > startTime)
	assert.Equal(t, "hostname-for-test", payload.Hostname)
	assert.Equal(t, 1234, payload.Metadata["test"])
}

func TestCapabilitiesFullAgentMetadataRefresh(t *testing.T) {
	for _, tt := range []struct {
		name         string
		capabilities *iainterface.Capabilities
		wantFetches  int
	}{
		{name: "absent capabilities", wantFetches: 1},
		{name: "zero capabilities", capabilities: &iainterface.Capabilities{}, wantFetches: 1},
		{name: "serverless capabilities", capabilities: iainterface.NewServerlessCapabilities(nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			originalSecurity, originalProcess := fetchSecurityConfig, fetchProcessConfig
			originalTrace, originalSystemProbe := fetchTraceConfig, fetchSystemProbeConfig
			t.Cleanup(func() {
				fetchSecurityConfig, fetchProcessConfig = originalSecurity, originalProcess
				fetchTraceConfig, fetchSystemProbeConfig = originalTrace, originalSystemProbe
			})
			fetches := map[string]int{}
			fetcher := func(name, remoteConfig string) func(pkgconfigmodel.Reader, ipc.HTTPClient) (string, error) {
				return func(_ pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
					fetches[name]++
					return remoteConfig, nil
				}
			}
			fetchSecurityConfig = fetcher("security", "compliance_config:\n  enabled: true")
			fetchProcessConfig = fetcher("process", "process_config:\n  process_collection:\n    enabled: true")
			fetchTraceConfig = fetcher("trace", "apm_config:\n  enabled: true")
			fetchSystemProbeConfig = fetcher("system-probe", "network_config:\n  enabled: true")

			var options []fx.Option
			if tt.capabilities != nil {
				options = append(options, fx.Supply(tt.capabilities))
			}
			if tt.wantFetches == 0 {
				options = append(options, fx.Decorate(func(ipc.HTTPClient) ipc.HTTPClient { return nil }))
			}
			p := getProvides(t, map[string]any{"inventories_configuration_enabled": true}, nil, options...)
			ia := p.Comp.(*inventoryagent)
			configDir := t.TempDir()
			configmock.New(t).SetConfigFile(filepath.Join(configDir, "datadog.yaml"))
			fleetDir := filepath.Join(configDir, "managed", "datadog-agent", "stable")
			if err := os.MkdirAll(fleetDir, 0700); err != nil {
				t.Fatal(err)
			}
			ia.Set("resource_id", "test-resource")
			ia.Set("flavor", "embedded-test")
			ia.Set("config_dd_url", "explicit-url")
			for index, value := range []string{"first", "second"} {
				ia.conf.Set("site", value+".example.com", pkgconfigmodel.SourceAgentRuntime)
				ia.conf.Set("logs_enabled", index == 0, pkgconfigmodel.SourceAgentRuntime)
				ia.conf.Set("config_id", value, pkgconfigmodel.SourceAgentRuntime)
				ia.conf.Set("fleet_layers", []string{value}, pkgconfigmodel.SourceAgentRuntime)
				applicationConfig := []byte("service: " + value)
				fleetConfig := []byte("service: fleet-" + value)
				for path, contents := range map[string][]byte{
					filepath.Join(configDir, "application_monitoring.yaml"): applicationConfig,
					filepath.Join(fleetDir, "application_monitoring.yaml"):  fleetConfig,
				} {
					if err := os.WriteFile(path, contents, 0600); err != nil {
						t.Fatal(err)
					}
				}
				payload := ia.getPayload().(*Payload)

				assert.Equal(t, version.AgentVersion, payload.Metadata["agent_version"])
				assert.Equal(t, version.AgentPackageVersion, payload.Metadata["package_version"])
				assert.Equal(t, ia.conf.StartTime().UnixMilli(), payload.Metadata["agent_startup_time_ms"])
				assert.Equal(t, "full", payload.Metadata["infrastructure_mode"])
				assert.Contains(t, payload.Metadata, "install_method_tool")
				assert.Equal(t, "embedded-test", payload.Metadata["flavor"])
				assert.Equal(t, "test-resource", payload.Metadata["resource_id"])
				assert.Equal(t, uuid.GetUUID(), payload.UUID)
				for _, field := range []string{"full_configuration", "agent_runtime_configuration"} {
					assert.Contains(t, payload.Metadata[field], "site: "+value+".example.com", field)
				}
				for name, field := range map[string]string{
					"security":     "feature_cspm_enabled",
					"process":      "feature_process_enabled",
					"trace":        "feature_apm_enabled",
					"system-probe": "feature_networks_enabled",
				} {
					assert.Equal(t, (index+1)*tt.wantFetches, fetches[name], name)
					if tt.wantFetches == 0 {
						assert.NotContains(t, payload.Metadata, field)
					} else {
						assert.Equal(t, true, payload.Metadata[field], field)
					}
				}
				// Local collectors are intentionally skipped too, even with populated inputs.
				for field, expected := range map[string]any{
					"config_site":                         value + ".example.com",
					"feature_logs_enabled":                index == 0,
					"fleet_policies_applied":              []string{value},
					"config_id":                           value,
					"application_monitoring_config":       applicationConfig,
					"application_monitoring_config_fleet": fleetConfig,
				} {
					if tt.wantFetches == 0 {
						assert.NotContains(t, payload.Metadata, field)
					} else {
						assert.Equal(t, expected, payload.Metadata[field], field)
					}
				}
				if tt.wantFetches == 0 {
					assert.Equal(t, "explicit-url", payload.Metadata["config_dd_url"], "skipping refresh must not filter explicitly set fields")
				} else {
					assert.Equal(t, ia.conf.GetString("dd_url"), payload.Metadata["config_dd_url"], "default refresh replaces explicitly set collector fields")
				}
			}
		})
	}
}

func TestCapabilitiesReadiness(t *testing.T) {
	for _, tt := range []struct {
		name         string
		capabilities *iainterface.Capabilities
		deferred     bool
	}{
		{name: "absent capabilities"},
		{name: "zero capabilities", capabilities: &iainterface.Capabilities{}},
		{name: "deferred capability", capabilities: &iainterface.Capabilities{DeferUntilReady: true}, deferred: true},
		{name: "serverless capabilities", capabilities: iainterface.NewServerlessCapabilities(nil), deferred: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			serializerMock := serializermock.NewMetricSerializer(t)
			options := []fx.Option{fx.Decorate(func(serializer.MetricSerializer) serializer.MetricSerializer {
				return serializerMock
			})}
			if tt.capabilities != nil {
				options = append(options, fx.Supply(tt.capabilities))
			}
			p := getProvides(t, map[string]any{"inventories_first_run_delay": 0}, nil, options...)
			ia := p.Comp.(*inventoryagent)
			require.NotNil(t, p.Provider.Callback, "a deferred provider must remain registered")
			if !tt.deferred {
				serializerMock.On("SendMetadata", mock.Anything).Return(nil).Twice()
			}

			// Exercise the published callback immediately after construction, before injection.
			assert.Equal(t, ia.MinInterval, p.Provider.Callback(t.Context()))
			p.Comp.Set("resource_id", "complete-resource")
			p.Comp.Submit()
			data, err := ia.GetAsJSON()
			if tt.deferred {
				assert.EqualError(t, err, "inventory metadata is not ready")
				assert.Nil(t, data)
				assert.True(t, ia.LastCollect.IsZero())
				assert.True(t, ia.RefreshTriggered())
				serializerMock.AssertNotCalled(t, "SendMetadata", mock.Anything)

				serializerMock.On("SendMetadata", mock.MatchedBy(func(payload *Payload) bool {
					return payload.Metadata["resource_id"] == "complete-resource"
				})).Return(nil).Once()
				p.Comp.SetReady(true)
				p.Comp.Submit()
				data, err = ia.GetAsJSON()
			}
			require.NoError(t, err)
			assert.Contains(t, string(data), `"resource_id": "complete-resource"`)
			assert.False(t, ia.LastCollect.IsZero())
			assert.False(t, ia.RefreshTriggered())
			serializerMock.AssertExpectations(t)
		})
	}
}

func TestCapabilitiesReadinessDoesNotEnableInventory(t *testing.T) {
	p := getProvides(t, map[string]any{"inventories_enabled": false}, nil,
		fx.Supply(iainterface.NewServerlessCapabilities(func() string {
			t.Fatal("disabled inventory must not resolve identity")
			return ""
		})))
	ia := p.Comp.(*inventoryagent)
	p.Comp.SetReady(true)
	p.Comp.Submit()
	assert.False(t, ia.Enabled)
	assert.Nil(t, p.Provider.Callback)
	assert.Nil(t, ia.MetadataProvider().Callback)
	assert.True(t, ia.LastCollect.IsZero())
	_, err := ia.GetAsJSON()
	assert.EqualError(t, err, "inventory metadata is disabled")
}

func TestCapabilitiesPayloadUUID(t *testing.T) {
	for _, tt := range []struct {
		name         string
		withResolver bool
		resolvedUUID string
	}{
		{name: "nil resolver"},
		{name: "empty override", withResolver: true},
		{name: "override", withResolver: true, resolvedUUID: "process-uuid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			originalGetUUID := uuid.GetUUID
			t.Cleanup(func() { uuid.GetUUID = originalGetUUID })
			hostUUIDCalls := 0
			uuid.GetUUID = func() string {
				hostUUIDCalls++
				return "host-uuid"
			}
			resolvedUUID := tt.resolvedUUID
			resolverCalls := 0
			var resolver func() string
			if tt.withResolver {
				resolver = func() string {
					resolverCalls++
					return resolvedUUID
				}
			}
			p := getProvides(t, nil, nil, fx.Supply(iainterface.NewServerlessCapabilities(resolver)))
			ia := p.Comp.(*inventoryagent)
			first := ia.getPayload().(*Payload)
			if resolvedUUID == "" {
				assert.Equal(t, "host-uuid", first.UUID)
				assert.Equal(t, 1, hostUUIDCalls)
			} else {
				assert.Equal(t, resolvedUUID, first.UUID)
				assert.Zero(t, hostUUIDCalls)
			}
			resolvedUUID = "next-instance-uuid"
			second := ia.getPayload().(*Payload)
			if tt.withResolver {
				assert.Equal(t, resolvedUUID, second.UUID)
				assert.Equal(t, 2, resolverCalls)
				if tt.resolvedUUID == "" {
					assert.Equal(t, 1, hostUUIDCalls)
				} else {
					assert.Zero(t, hostUUIDCalls)
				}
			} else {
				assert.Equal(t, "host-uuid", second.UUID)
				assert.Equal(t, 2, hostUUIDCalls)
			}
		})
	}
}

func TestInitDataErrorInstallInfo(t *testing.T) {
	defer func() { installinfoGet = installinfo.Get }()
	installinfoGet = func(config.Reader) (*installinfo.InstallInfo, error) {
		return nil, errors.New("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)

	ia.initData()
	assert.Equal(t, "undefined", ia.data["install_method_tool"])
	assert.Equal(t, "", ia.data["install_method_tool_version"])
	assert.Equal(t, "", ia.data["install_method_installer_version"])

	installinfoGet = func(config.Reader) (*installinfo.InstallInfo, error) {
		return &installinfo.InstallInfo{
			Tool:             "test_tool",
			ToolVersion:      "1.2.3",
			InstallerVersion: "4.5.6",
		}, nil
	}

	ia.initData()
	assert.Equal(t, "test_tool", ia.data["install_method_tool"])
	assert.Equal(t, "1.2.3", ia.data["install_method_tool_version"])
	assert.Equal(t, "4.5.6", ia.data["install_method_installer_version"])
}

func TestInitData(t *testing.T) {
	sysprobeOverrides := map[string]any{
		"dynamic_instrumentation.enabled":                      true,
		"remote_configuration.enabled":                         true,
		"runtime_security_config.enabled":                      true,
		"event_monitoring_config.network.enabled":              true,
		"runtime_security_config.activity_dump.enabled":        true,
		"runtime_security_config.remote_configuration.enabled": true,
		"network_config.enabled":                               true,
		"traceroute.enabled":                                   true,
		"service_monitoring_config.enable_http_monitoring":     true,
		"service_monitoring_config.tls.native.enabled":         true,
		"service_monitoring_config.enabled":                    true,
		"service_monitoring_config.http2.enabled":              true,
		"service_monitoring_config.kafka.enabled":              true,
		"service_monitoring_config.postgres.enabled":           true,
		"service_monitoring_config.redis.enabled":              true,
		"service_monitoring_config.tls.istio.enabled":          true,
		"service_monitoring_config.tls.go.enabled":             true,
		"discovery.enabled":                                    true,
		"gpu_monitoring.enabled":                               true,
		"system_probe_config.enable_tcp_queue_length":          true,
		"system_probe_config.enable_oom_kill":                  true,
		"windows_crash_detection.enabled":                      true,
		"system_probe_config.enable_co_re":                     true,
		"system_probe_config.enable_runtime_compiler":          true,
		"system_probe_config.enable_kernel_header_download":    true,
		"system_probe_config.allow_prebuilt_fallback":          true,
		"system_probe_config.telemetry_enabled":                true,
		"system_probe_config.max_conns_per_message":            10,
		"system_probe_config.disable_ipv6":                     false,
		"network_config.collect_tcp_v4":                        true,
		"network_config.collect_tcp_v6":                        true,
		"network_config.collect_udp_v4":                        true,
		"network_config.collect_udp_v6":                        true,
		"network_config.enable_protocol_classification":        true,
		"network_config.enable_gateway_lookup":                 true,
		"network_config.enable_root_netns":                     true,
	}

	overrides := map[string]any{
		"language_detection.enabled":       true,
		"apm_config.apm_dd_url":            "http://name:sekrit@someintake.example.com/",
		"dd_url":                           "http://name:sekrit@someintake.example.com/",
		"logs_config.logs_dd_url":          "http://name:sekrit@someintake.example.com/",
		"logs_config.socks5_proxy_address": "http://name:sekrit@proxy.example.com/",
		"proxy.no_proxy":                   []string{"http://noprox.example.com", "http://name:sekrit@proxy.example.com/"},
		"process_config.process_dd_url":    "http://name:sekrit@someintake.example.com/",
		"proxy.http":                       "http://name:sekrit@proxy.example.com/",
		"proxy.https":                      "https://name:sekrit@proxy.example.com/",
		"site":                             "test",
		"eks_fargate":                      true,
		"synthetics.collector.enabled":     true,

		"fips.enabled":                                true,
		"logs_enabled":                                true,
		"compliance_config.enabled":                   true,
		"compliance_config.host_benchmarks.enabled":   true,
		"apm_config.enabled":                          true,
		"ec2_prefer_imdsv2":                           true,
		"process_config.container_collection.enabled": true,
		"remote_configuration.enabled":                true,
		"remote_updates":                              true,
		"process_config.process_collection.enabled":   true,
		"container_image.enabled":                     true,
		"network_path.remote_config.enabled":          true,
		"network_path.connections_monitoring.enabled": true,
		"sbom.enabled":                                true,
		"sbom.container_image.enabled":                true,
		"sbom.host.enabled":                           true,
		"infrastructure_mode":                         "basic",
	}
	ia := getTestInventoryPayload(t, overrides, sysprobeOverrides)
	ia.refreshMetadata()
	isFips, err := fips.Enabled()
	assert.Nil(t, err)

	expected := map[string]any{
		"agent_version":                    version.AgentVersion,
		"agent_startup_time_ms":            ia.conf.StartTime().UnixMilli(),
		"flavor":                           flavor.GetFlavor(),
		"fips_mode":                        isFips,
		"config_apm_dd_url":                "http://name:********@someintake.example.com/",
		"config_dd_url":                    "http://name:********@someintake.example.com/",
		"config_site":                      "test",
		"config_logs_dd_url":               "http://name:********@someintake.example.com/",
		"config_logs_socks5_proxy_address": "http://name:********@proxy.example.com/",
		"config_process_dd_url":            "http://name:********@someintake.example.com/",
		"config_proxy_http":                "http://name:********@proxy.example.com/",
		"config_proxy_https":               "https://name:********@proxy.example.com/",
		"config_eks_fargate":               true,

		"feature_process_language_detection_enabled":          true,
		"feature_logs_enabled":                                true,
		"feature_cspm_enabled":                                true,
		"feature_cspm_host_benchmarks_enabled":                true,
		"feature_apm_enabled":                                 true,
		"feature_imdsv2_enabled":                              true,
		"feature_processes_container_enabled":                 true,
		"feature_remote_configuration_enabled":                true,
		"feature_remote_updates_enabled":                      true,
		"feature_process_enabled":                             true,
		"feature_container_images_enabled":                    true,
		"feature_network_path_remote_config_enabled":          true,
		"feature_network_path_connections_monitoring_enabled": true,

		"feature_dynamic_instrumentation_enabled":      true,
		"feature_cws_enabled":                          true,
		"feature_cws_network_enabled":                  true,
		"feature_cws_security_profiles_enabled":        true,
		"feature_cws_remote_config_enabled":            true,
		"feature_csm_vm_containers_enabled":            true,
		"feature_csm_vm_hosts_enabled":                 true,
		"feature_networks_enabled":                     true,
		"feature_networks_http_enabled":                true,
		"feature_networks_https_enabled":               true,
		"feature_traceroute_enabled":                   true,
		"feature_synthetics_collector_enabled":         true,
		"feature_usm_enabled":                          true,
		"feature_usm_kafka_enabled":                    true,
		"feature_usm_postgres_enabled":                 true,
		"feature_usm_redis_enabled":                    true,
		"feature_usm_http2_enabled":                    true,
		"feature_usm_istio_enabled":                    true,
		"feature_usm_go_tls_enabled":                   true,
		"feature_discovery_enabled":                    true,
		"feature_gpu_monitoring_enabled":               true,
		"feature_tcp_queue_length_enabled":             true,
		"feature_oom_kill_enabled":                     true,
		"feature_windows_crash_detection_enabled":      true,
		"system_probe_core_enabled":                    true,
		"system_probe_runtime_compilation_enabled":     true,
		"system_probe_kernel_headers_download_enabled": true,
		"system_probe_prebuilt_fallback_enabled":       true,
		"system_probe_telemetry_enabled":               true,
		"system_probe_track_tcp_4_connections":         true,
		"system_probe_track_tcp_6_connections":         true,
		"system_probe_track_udp_4_connections":         true,
		"system_probe_track_udp_6_connections":         true,
		"system_probe_protocol_classification_enabled": true,
		"system_probe_gateway_lookup_enabled":          true,
		"system_probe_root_namespace_enabled":          true,
		"system_probe_max_connections_per_message":     10,
		"infrastructure_mode":                          "basic",
	}

	if !kernel.IsIPv6Enabled() {
		expected["system_probe_track_tcp_6_connections"] = false
		expected["system_probe_track_udp_6_connections"] = false
	}

	// Redis may be disabled by adjust_usm.go on kernels < 5.4
	if !sysprobecfg.RedisMonitoringSupported() {
		expected["feature_usm_redis_enabled"] = false
	}

	// HTTP2 may be disabled by adjust_usm.go on kernels < 5.2
	expected["feature_usm_http2_enabled"] = sysprobecfg.HTTP2MonitoringSupported()

	for name, value := range expected {
		assert.Equal(t, value, ia.data[name], "value for '%s' is wrong", name)
	}

	assert.ElementsMatch(t, []string{"http://noprox.example.com", "http://name:********@proxy.example.com/"}, ia.data["config_no_proxy"])
}

func TestGet(t *testing.T) {
	ia := getTestInventoryPayload(t, nil, nil)

	ia.Set("test", 1234)

	p := ia.Get()
	assert.Equal(t, 1234, p["test"])

	// verify that the return map is a copy
	p["test"] = 21
	assert.Equal(t, 1234, ia.data["test"])
}

func TestFlareProviderFilename(t *testing.T) {
	ia := getTestInventoryPayload(t, nil, nil)
	assert.Equal(t, "agent.json", ia.FlareFileName)
}

func TestConfigRefresh(t *testing.T) {
	cfg := configmock.New(t)
	ia := getTestInventoryPayload(t, nil, nil)

	assert.False(t, ia.RefreshTriggered())
	cfg.Set("inventories_max_interval", 10*60, pkgconfigmodel.SourceAgentRuntime)
	assert.True(t, ia.RefreshTriggered())
}

func TestStatusHeaderProvider(t *testing.T) {
	ret := getProvides(t, nil, nil)

	headerStatusProvider := ret.StatusHeaderProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerStatusProvider.JSON(false, stats)

			keys := slices.Collect(maps.Keys(stats))

			assert.Contains(t, keys, "agent_metadata")
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerStatusProvider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerStatusProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestFetchSecurityAgent(t *testing.T) {
	defer func() {
		fetchSecurityConfig = configFetcher.SecurityAgentConfig
	}()

	fetchSecurityConfig = func(config pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
		// test that the agent config was passed and not the system-probe config.
		assert.False(
			t,
			config.IsKnown("system_probe_config.sysprobe_socket"),
			"wrong configuration received for security-agent fetcher",
		)
		assert.True(
			t,
			config.IsKnown("hostname"),
			"wrong configuration received for security-agent fetcher",
		)

		return "", errors.New("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchSecurityAgentMetadata()

	assert.False(t, ia.data["feature_cspm_enabled"].(bool))
	assert.False(t, ia.data["feature_cspm_host_benchmarks_enabled"].(bool))

	fetchSecurityConfig = func(_ pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
		return `compliance_config:
  enabled: true
  host_benchmarks:
    enabled: true
`, nil
	}

	ia.fetchSecurityAgentMetadata()

	assert.True(t, ia.data["feature_cspm_enabled"].(bool))
	assert.True(t, ia.data["feature_cspm_host_benchmarks_enabled"].(bool))
}

func TestFetchProcessAgent(t *testing.T) {
	defer func(original func(cfg pkgconfigmodel.Reader, client ipc.HTTPClient) (string, error)) {
		fetchProcessConfig = original
	}(fetchProcessConfig)

	fetchProcessConfig = func(config pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
		// test that the agent config was passed and not the system-probe config.
		assert.False(
			t,
			config.IsKnown("system_probe_config.sysprobe_socket"),
			"wrong configuration received for process-agent fetcher",
		)
		assert.True(
			t,
			config.IsKnown("hostname"),
			"wrong configuration received for security-agent fetcher",
		)

		return "", errors.New("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchProcessAgentMetadata()

	assert.False(t, ia.data["feature_process_enabled"].(bool))
	assert.False(t, ia.data["feature_process_language_detection_enabled"].(bool))
	// default to true in the process agent configuration
	assert.True(t, ia.data["feature_processes_container_enabled"].(bool))

	fetchProcessConfig = func(_ pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
		return `
process_config:
  process_collection:
    enabled: true
  container_collection:
    enabled: true
language_detection:
  enabled: true
`, nil
	}

	ia.fetchProcessAgentMetadata()

	assert.True(t, ia.data["feature_process_enabled"].(bool))
	assert.True(t, ia.data["feature_processes_container_enabled"].(bool))
	assert.True(t, ia.data["feature_process_language_detection_enabled"].(bool))
}

func TestFetchTraceAgent(t *testing.T) {
	defer func() {
		fetchTraceConfig = configFetcher.TraceAgentConfig
	}()
	fetchTraceConfig = func(config pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
		// test that the agent config was passed and not the system-probe config.
		assert.False(
			t,
			config.IsKnown("system_probe_config.sysprobe_socket"),
			"wrong configuration received for trace-agent fetcher",
		)
		assert.True(
			t,
			config.IsKnown("hostname"),
			"wrong configuration received for security-agent fetcher",
		)

		return "", errors.New("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchTraceAgentMetadata()

	assert.True(t, ia.data["feature_apm_enabled"].(bool))
	assert.Equal(t, "", ia.data["config_apm_dd_url"].(string))

	fetchTraceConfig = func(_ pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
		return `
apm_config:
  enabled: true
  apm_dd_url: "https://user:password@some_url_for_trace"
`, nil
	}

	ia.fetchTraceAgentMetadata()
	assert.True(t, ia.data["feature_apm_enabled"].(bool))
	assert.Equal(t, "https://user:********@some_url_for_trace", ia.data["config_apm_dd_url"].(string))
}

func TestFetchSystemProbeAgent(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("system-probe does not support darwin")
	}

	defer func() {
		fetchSystemProbeConfig = sysprobeConfigFetcher.SystemProbeConfig
	}()
	fetchSystemProbeConfig = func(config pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
		// test that the system-probe config was passed and not the agent config
		assert.True(
			t,
			config.IsConfigured("system_probe_config.sysprobe_socket"),
			"wrong configuration received for system-probe fetcher",
		)
		assert.False(
			t,
			config.IsConfigured("hostname"),
			"wrong configuration received for security-agent fetcher",
		)

		return "", errors.New("some error")
	}

	isPrebuiltDeprecated := prebuilt.IsDeprecated()

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchSystemProbeMetadata()

	// We test default value when the system-probe could not be contacted
	assert.False(t, ia.data["feature_cws_enabled"].(bool))
	assert.True(t, ia.data["feature_cws_network_enabled"].(bool))
	assert.False(t, ia.data["feature_cws_security_profiles_enabled"].(bool))
	assert.True(t, ia.data["feature_cws_remote_config_enabled"].(bool))
	assert.False(t, ia.data["feature_networks_enabled"].(bool))
	assert.True(t, ia.data["feature_networks_http_enabled"].(bool))
	assert.True(t, ia.data["feature_networks_https_enabled"].(bool))
	assert.False(t, ia.data["feature_traceroute_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_kafka_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_postgres_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_redis_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_go_tls_enabled"].(bool))
	assert.Equal(t, runtime.GOOS == "linux", ia.data["feature_discovery_enabled"].(bool))
	assert.False(t, ia.data["feature_gpu_monitoring_enabled"].(bool))
	assert.False(t, ia.data["feature_tcp_queue_length_enabled"].(bool))
	assert.False(t, ia.data["feature_oom_kill_enabled"].(bool))
	assert.False(t, ia.data["feature_windows_crash_detection_enabled"].(bool))
	assert.True(t, ia.data["system_probe_core_enabled"].(bool))
	assert.False(t, ia.data["system_probe_runtime_compilation_enabled"].(bool))
	assert.False(t, ia.data["system_probe_kernel_headers_download_enabled"].(bool))
	if runtime.GOOS == "linux" {
		assert.Equal(t, !isPrebuiltDeprecated, ia.data["system_probe_prebuilt_fallback_enabled"].(bool))
	}
	assert.False(t, ia.data["system_probe_telemetry_enabled"].(bool))
	assert.Equal(t, 600, ia.data["system_probe_max_connections_per_message"].(int))
	assert.True(t, ia.data["system_probe_track_tcp_4_connections"].(bool))
	assert.True(t, ia.data["system_probe_track_udp_4_connections"].(bool))
	if !kernel.IsIPv6Enabled() {
		assert.False(t, ia.data["system_probe_track_tcp_6_connections"].(bool))
		assert.False(t, ia.data["system_probe_track_udp_6_connections"].(bool))
	} else {
		assert.True(t, ia.data["system_probe_track_tcp_6_connections"].(bool))
		assert.True(t, ia.data["system_probe_track_udp_6_connections"].(bool))
	}
	assert.True(t, ia.data["system_probe_protocol_classification_enabled"].(bool))
	assert.True(t, ia.data["system_probe_gateway_lookup_enabled"].(bool))
	assert.True(t, ia.data["system_probe_root_namespace_enabled"].(bool))
	assert.False(t, ia.data["feature_dynamic_instrumentation_enabled"].(bool))

	// Testing an inventoryagent without system-probe object
	p := NewComponent(
		makeRequires(fxutil.Test[testDeps](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			sysprobeconfig.NoneModule(),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
			hostnameimpl.MockModule(),
		)),
	)
	ia = p.Comp.(*inventoryagent)
	ia.fetchSystemProbeMetadata()

	assert.False(t, ia.data["feature_cws_enabled"].(bool))
	assert.False(t, ia.data["feature_cws_network_enabled"].(bool))
	assert.False(t, ia.data["feature_cws_security_profiles_enabled"].(bool))
	assert.False(t, ia.data["feature_cws_remote_config_enabled"].(bool))
	assert.False(t, ia.data["feature_networks_enabled"].(bool))
	assert.False(t, ia.data["feature_networks_http_enabled"].(bool))
	assert.False(t, ia.data["feature_networks_https_enabled"].(bool))
	assert.False(t, ia.data["feature_traceroute_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_kafka_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_postgres_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_go_tls_enabled"].(bool))
	assert.False(t, ia.data["feature_discovery_enabled"].(bool))
	assert.False(t, ia.data["feature_gpu_monitoring_enabled"].(bool))
	assert.False(t, ia.data["feature_tcp_queue_length_enabled"].(bool))
	assert.False(t, ia.data["feature_oom_kill_enabled"].(bool))
	assert.False(t, ia.data["feature_windows_crash_detection_enabled"].(bool))
	assert.False(t, ia.data["system_probe_core_enabled"].(bool))
	assert.False(t, ia.data["system_probe_runtime_compilation_enabled"].(bool))
	assert.False(t, ia.data["system_probe_kernel_headers_download_enabled"].(bool))
	assert.False(t, ia.data["system_probe_prebuilt_fallback_enabled"].(bool))
	assert.False(t, ia.data["system_probe_telemetry_enabled"].(bool))
	assert.Equal(t, 0, ia.data["system_probe_max_connections_per_message"].(int))
	assert.False(t, ia.data["system_probe_track_tcp_4_connections"].(bool))
	assert.False(t, ia.data["system_probe_track_tcp_6_connections"].(bool))
	assert.False(t, ia.data["system_probe_track_udp_4_connections"].(bool))
	assert.False(t, ia.data["system_probe_track_udp_6_connections"].(bool))
	assert.False(t, ia.data["system_probe_protocol_classification_enabled"].(bool))
	assert.False(t, ia.data["system_probe_gateway_lookup_enabled"].(bool))
	assert.False(t, ia.data["system_probe_root_namespace_enabled"].(bool))
	assert.False(t, ia.data["feature_dynamic_instrumentation_enabled"].(bool))

	// Testing an inventoryagent where we can contact the system-probe process
	fetchSystemProbeConfig = func(_ pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
		return `
runtime_security_config:
  enabled: true
  activity_dump:
    enabled: true
  remote_configuration:
    enabled: true

event_monitoring_config:
  network:
    enabled: true

network_config:
  enabled: true
  collect_tcp_v4: true
  collect_tcp_v6: true
  collect_udp_v4: true
  collect_udp_v6: true
  enable_protocol_classification: true
  enable_gateway_lookup: true
  enable_root_netns: true

traceroute:
  enabled: true

service_monitoring_config:
  http:
    enabled: true
  tls:
    native:
      enabled: true
    java:
      enabled: true
    istio:
      enabled: true
    go:
      enabled: true
  enabled: true
  kafka:
    enabled: true
  postgres:
    enabled: true
  redis:
    enabled: true
  http2:
    enabled: true
  enable_http_stats_by_status_code: true

discovery:
  enabled: true

windows_crash_detection:
  enabled: true

system_probe_config:
  enable_tcp_queue_length: true
  enable_oom_kill: true
  enable_co_re: true
  enable_runtime_compiler: true
  enable_kernel_header_download: true
  allow_prebuilt_fallback: true
  telemetry_enabled: true
  max_conns_per_message: 123

dynamic_instrumentation:
  enabled: true

gpu_monitoring:
  enabled: true
`, nil
	}

	ia = getTestInventoryPayload(t, nil, nil)
	ia.fetchSystemProbeMetadata()

	assert.True(t, ia.data["feature_cws_enabled"].(bool))
	assert.True(t, ia.data["feature_cws_network_enabled"].(bool))
	assert.True(t, ia.data["feature_cws_security_profiles_enabled"].(bool))
	assert.True(t, ia.data["feature_cws_remote_config_enabled"].(bool))
	assert.True(t, ia.data["feature_networks_enabled"].(bool))
	assert.True(t, ia.data["feature_networks_http_enabled"].(bool))
	assert.True(t, ia.data["feature_networks_https_enabled"].(bool))
	assert.True(t, ia.data["feature_traceroute_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_kafka_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_postgres_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_redis_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_go_tls_enabled"].(bool))
	assert.True(t, ia.data["feature_discovery_enabled"].(bool))
	assert.True(t, ia.data["feature_gpu_monitoring_enabled"].(bool))
	assert.True(t, ia.data["feature_tcp_queue_length_enabled"].(bool))
	assert.True(t, ia.data["feature_oom_kill_enabled"].(bool))
	assert.True(t, ia.data["feature_windows_crash_detection_enabled"].(bool))
	assert.True(t, ia.data["system_probe_core_enabled"].(bool))
	assert.True(t, ia.data["system_probe_runtime_compilation_enabled"].(bool))
	assert.True(t, ia.data["system_probe_kernel_headers_download_enabled"].(bool))
	assert.True(t, ia.data["system_probe_prebuilt_fallback_enabled"].(bool))
	assert.True(t, ia.data["system_probe_telemetry_enabled"].(bool))
	assert.Equal(t, 123, ia.data["system_probe_max_connections_per_message"].(int))
	assert.True(t, ia.data["system_probe_track_tcp_4_connections"].(bool))
	assert.True(t, ia.data["system_probe_track_udp_4_connections"].(bool))
	assert.True(t, ia.data["system_probe_track_tcp_6_connections"].(bool))
	assert.True(t, ia.data["system_probe_track_udp_6_connections"].(bool))
	assert.True(t, ia.data["system_probe_protocol_classification_enabled"].(bool))
	assert.True(t, ia.data["system_probe_gateway_lookup_enabled"].(bool))
	assert.True(t, ia.data["system_probe_root_namespace_enabled"].(bool))
	assert.True(t, ia.data["feature_dynamic_instrumentation_enabled"].(bool))
}

func TestFeatureGPUMonitoringEnabled(t *testing.T) {
	original := fetchSystemProbeConfig
	t.Cleanup(func() { fetchSystemProbeConfig = original })

	t.Run("core gpu.enabled without system-probe module", func(t *testing.T) {
		fetchSystemProbeConfig = func(_ pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
			return "gpu_monitoring:\n  enabled: false\n", nil
		}
		ia := getTestInventoryPayload(t, map[string]any{"gpu.enabled": true}, nil)
		ia.fetchSystemProbeMetadata()
		assert.True(t, ia.data["feature_gpu_monitoring_enabled"].(bool))
	})

	t.Run("system-probe gpu_monitoring.enabled without core check", func(t *testing.T) {
		fetchSystemProbeConfig = func(_ pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
			return "", errors.New("system-probe unavailable")
		}
		ia := getTestInventoryPayload(t, map[string]any{"gpu.enabled": false}, map[string]any{"gpu_monitoring.enabled": true})
		ia.fetchSystemProbeMetadata()
		assert.True(t, ia.data["feature_gpu_monitoring_enabled"].(bool))
	})

	t.Run("neither enabled", func(t *testing.T) {
		fetchSystemProbeConfig = func(_ pkgconfigmodel.Reader, _ ipc.HTTPClient) (string, error) {
			return "", errors.New("system-probe unavailable")
		}
		ia := getTestInventoryPayload(t, map[string]any{"gpu.enabled": false}, map[string]any{"gpu_monitoring.enabled": false})
		ia.fetchSystemProbeMetadata()
		assert.False(t, ia.data["feature_gpu_monitoring_enabled"].(bool))
	})

	t.Run("core gpu.enabled when system-probe config is not loaded", func(t *testing.T) {
		p := NewComponent(
			makeRequires(fxutil.Test[testDeps](
				t,
				fx.Provide(func() log.Component { return logmock.New(t) }),
				fx.Provide(func() config.Component {
					return config.NewMockWithOverrides(t, map[string]any{"gpu.enabled": true})
				}),
				sysprobeconfig.NoneModule(),
				fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
				fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
				fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
				hostnameimpl.MockModule(),
			)),
		)
		ia := p.Comp.(*inventoryagent)
		ia.fetchSystemProbeMetadata()
		assert.True(t, ia.data["feature_gpu_monitoring_enabled"].(bool))
	})
}

func TestFetchFleet(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"config_id": "my-config",
	}, nil)
	ia.fetchFleetMetadata()
	assert.Equal(t, "my-config", ia.data["config_id"].(string))
}

func TestFetchSecretsNoComponent(t *testing.T) {
	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchSecretsMetadata()
	assert.Equal(t, map[string]interface{}{
		"paths":          []string{},
		"handles":        []string{},
		"scrubbed":       []string{},
		"hash_secret_id": map[string]string{},
	}, ia.data["enc_info"])
}

func TestFetchSecrets(t *testing.T) {
	secretsComp := secretsmock.New(t)
	secretsComp.SetPathsAndHandles(
		[]string{"api_key", "snmp_traps.community_strings.0", "snmp_traps.community_strings.1"},
		[]string{"my_key", "secret_one", "secret_two"},
	)
	// "secret_two" has no resolved value and is omitted from the hashes
	secretsComp.SetSecrets(map[string]string{"my_key": "my_key_value", "secret_one": "secret_one_value"})
	p := getProvides(t, map[string]any{
		// covered by a secret path, so not reported as scrubbed
		"api_key": "aaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb",
		// known sensitive key name
		"network_devices.snmp_traps.users": []any{
			map[string]any{"user": "user1", "authKey": "my-auth-key", "authProtocol": "sha"},
		},
		// raw credential written in a setting that is not known to be sensitive
		"dd_url": "https://user:secret@example.com",
	}, nil, fx.Provide(func() secrets.Component { return secretsComp }))
	ia := p.Comp.(*inventoryagent)

	ia.fetchSecretsMetadata()
	assert.Equal(t, map[string]interface{}{
		"paths":   []string{"api_key", "snmp_traps.community_strings.0", "snmp_traps.community_strings.1"},
		"handles": []string{"my_key", "secret_one", "secret_two"},
		"hash_secret_id": map[string]string{
			"my_key":     "234c1d086201f04697450c7bcbe1e75a927ff021529b3a9eb785b40e9e0339fa",
			"secret_one": "c9941a386f3401c35efab67c9a302b4f13317d0d018ae6697b531dce994a8008",
		},
		"scrubbed": []string{"authKey", "dd_url"},
	}, ia.data["enc_info"])
}

func TestFetchSecretsOmitted(t *testing.T) {
	secretsComp := secretsmock.New(t)
	secretsComp.SetPathsAndHandles([]string{"api_key"}, []string{"my_key"})
	secretsComp.SetSecrets(map[string]string{"my_key": "my_key_value"})
	p := getProvides(t, map[string]any{"metadata_skip_secrets": true}, nil, fx.Provide(func() secrets.Component { return secretsComp }))
	ia := p.Comp.(*inventoryagent)

	ia.data["enc_info"] = "stale"
	ia.fetchSecretsMetadata()
	assert.NotContains(t, ia.data, "enc_info")
}

func TestFlareScrubsSecretHashes(t *testing.T) {
	secretsComp := secretsmock.New(t)
	secretsComp.SetPathsAndHandles([]string{"api_key"}, []string{"my_key"})
	secretsComp.SetSecrets(map[string]string{"my_key": "my_key_value"})
	p := getProvides(t, map[string]any{"inventories_enabled": true}, nil, fx.Provide(func() secrets.Component { return secretsComp }))

	fb := flarehelpers.NewFlareBuilderMock(t, false)
	require.NoError(t, p.FlareProvider.FlareFiller.Callback(t.Context(), fb))

	content, err := os.ReadFile(filepath.Join(fb.Root, "metadata", "inventory", "agent.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"hash_secret_id": "********"`)
	assert.Contains(t, string(content), `"my_key"`)
	assert.NotContains(t, string(content), "234c1d086201f04697450c7bcbe1e75a927ff021529b3a9eb785b40e9e0339fa")
}

func TestShowMetadataScrubsSecretHashes(t *testing.T) {
	secretsComp := secretsmock.New(t)
	secretsComp.SetPathsAndHandles([]string{"api_key"}, []string{"my_key"})
	secretsComp.SetSecrets(map[string]string{"my_key": "my_key_value"})
	p := getProvides(t, map[string]any{"inventories_enabled": true}, nil, fx.Provide(func() secrets.Component { return secretsComp }))
	ia := p.Comp.(*inventoryagent)

	rec := httptest.NewRecorder()
	ia.writePayloadAsJSON(rec, httptest.NewRequest(http.MethodGet, "/metadata/inventory-agent", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"hash_secret_id": "********"`)
	assert.Contains(t, rec.Body.String(), `"my_key"`)
	assert.NotContains(t, rec.Body.String(), "234c1d086201f04697450c7bcbe1e75a927ff021529b3a9eb785b40e9e0339fa")
}

func TestStatusOmitsSecretHashes(t *testing.T) {
	secretsComp := secretsmock.New(t)
	secretsComp.SetPathsAndHandles([]string{"api_key"}, []string{"my_key"})
	secretsComp.SetSecrets(map[string]string{"my_key": "my_key_value"})
	p := getProvides(t, nil, nil, fx.Provide(func() secrets.Component { return secretsComp }))
	ia := p.Comp.(*inventoryagent)
	ia.fetchSecretsMetadata()

	stats := map[string]interface{}{}
	require.NoError(t, ia.JSON(false, stats))
	encInfo := stats["agent_metadata"].(map[string]interface{})["enc_info"].(map[string]interface{})
	assert.NotContains(t, encInfo, "hash_secret_id")
	assert.Equal(t, []string{"my_key"}, encInfo["handles"])

	text := new(bytes.Buffer)
	require.NoError(t, ia.Text(false, text))
	html := new(bytes.Buffer)
	require.NoError(t, ia.HTML(false, html))
	for _, out := range []string{text.String(), html.String()} {
		assert.Contains(t, out, "my_key")
		assert.NotContains(t, out, "hash_secret_id")
		assert.NotContains(t, out, "234c1d086201f04697450c7bcbe1e75a927ff021529b3a9eb785b40e9e0339fa")
	}

	// The payload sent to the backend still carries the hashes
	assert.Contains(t, ia.data["enc_info"], "hash_secret_id")
}

func TestFilterScrubbedKeys(t *testing.T) {
	tests := []struct {
		name         string
		scrubbedKeys []string
		paths        []string
		expected     []string
	}{
		{
			name:         "empty",
			scrubbedKeys: nil,
			paths:        nil,
			expected:     []string{},
		},
		{
			name:         "top-level setting",
			scrubbedKeys: []string{"dd_url", "client_secret"},
			paths:        []string{"dd_url"},
			expected:     []string{"client_secret"},
		},
		{
			name:         "nested setting",
			scrubbedKeys: []string{"https", "client_secret"},
			paths:        []string{"proxy.https"},
			expected:     []string{"client_secret"},
		},
		{
			name:         "slice index uses the parent key",
			scrubbedKeys: []string{"community_strings", "client_secret"},
			paths:        []string{"snmp_traps.community_strings.0", "snmp_traps.community_strings.1"},
			expected:     []string{"client_secret"},
		},
		{
			// integration configs use their digest as origin, and paths relative to the resolved section
			name:         "nested setting from an integration config",
			scrubbedKeys: []string{"password", "client_secret"},
			paths:        []string{"a3f9c01b2d4e5f67:auth.password"},
			expected:     []string{"client_secret"},
		},
		{
			name:         "top-level setting from an integration config",
			scrubbedKeys: []string{"token", "client_secret"},
			paths:        []string{"a3f9c01b2d4e5f67:token"},
			expected:     []string{"client_secret"},
		},
		{
			name:         "partial key names are not filtered",
			scrubbedKeys: []string{"proxy", "api"},
			paths:        []string{"proxy.https", "logs_config.api_key"},
			expected:     []string{"proxy", "api"},
		},
		{
			name:         "duplicates are removed",
			scrubbedKeys: []string{"password", "client_secret", "client_secret", "password"},
			paths:        []string{"password"},
			expected:     []string{"client_secret"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, filterScrubbedKeys(tt.scrubbedKeys, tt.paths))
		})
	}
}

func TestGetProvidedConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	payload := ia.getPayload().(*Payload)

	// No configuration should be in the payload
	assert.NotContains(t, payload.Metadata, "full_configuration")
	assert.NotContains(t, payload.Metadata, "provided_configuration")
	assert.NotContains(t, payload.Metadata, "file_configuration")
	assert.NotContains(t, payload.Metadata, "environment_variable_configuration")
	assert.NotContains(t, payload.Metadata, "agent_runtime_configuration")
	assert.NotContains(t, payload.Metadata, "remote_configuration")
	assert.NotContains(t, payload.Metadata, "cli_configuration")
	assert.NotContains(t, payload.Metadata, "source_local_configuration")
	assert.Empty(t, payload.FilesMetadata)
}

func TestGetPayloadFilesMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	configmock.New(t).SetConfigFile(filepath.Join(tmpDir, "datadog.yaml"))

	originalFetchSystemProbeFilename := fetchSystemProbeFilename
	defer func() {
		fetchSystemProbeFilename = originalFetchSystemProbeFilename
	}()
	fetchSystemProbeFilename = func(_ option.Option[sysprobeconfig.Component]) string {
		return filepath.Join(tmpDir, "system-probe.yaml")
	}

	configFiles := map[string]string{
		filepath.Join(tmpDir, "datadog.yaml"): `# comments are not safe to report
second: 2
first: "preserve this style"
unknown_setting:
  nested: untouched
api_key: datadog-secret
`,
		filepath.Join(tmpDir, "system-probe.yaml"):           "network_config:\n  enabled: true\nauth_token: system-probe-secret\n",
		filepath.Join(tmpDir, "security-agent.yaml"):         "runtime_security_config:\n  enabled: true\npassword: security-secret\n",
		filepath.Join(tmpDir, "application_monitoring.yaml"): "tracing:\n  enabled: true\napi_key: application-secret\n",
	}
	for path, contents := range configFiles {
		require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
	}

	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	payload := ia.getPayload().(*Payload)
	require.Len(t, payload.FilesMetadata, len(configFiles))
	for path := range configFiles {
		rawfile, found := payload.FilesMetadata[path]
		require.True(t, found)
		assert.NotContains(t, rawfile.RawConfig, "-secret")

		hash := sha256.Sum256([]byte(rawfile.RawConfig))
		assert.Equal(t, hex.EncodeToString(hash[:]), rawfile.Hash)
	}

	datadogRawConfig := payload.FilesMetadata[filepath.Join(tmpDir, "datadog.yaml")].RawConfig
	assert.Less(t, strings.Index(datadogRawConfig, "second:"), strings.Index(datadogRawConfig, "first:"))
	assert.Contains(t, datadogRawConfig, `first: "preserve this style"`)
	assert.Contains(t, datadogRawConfig, "unknown_setting:\n  nested: untouched")
	assert.NotContains(t, datadogRawConfig, "comments are not safe to report")
}

func TestGetPayloadFilesMetadataSkipsUnsafeAndMissingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	datadogConfigPath := filepath.Join(tmpDir, "datadog.yaml")
	configmock.New(t).SetConfigFile(datadogConfigPath)
	require.NoError(t, os.WriteFile(datadogConfigPath, []byte("api_key: [invalid"), 0600))

	originalFetchSystemProbeFilename := fetchSystemProbeFilename
	defer func() {
		fetchSystemProbeFilename = originalFetchSystemProbeFilename
	}()
	fetchSystemProbeFilename = func(_ option.Option[sysprobeconfig.Component]) string {
		return filepath.Join(tmpDir, "system-probe.yaml")
	}

	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	payload := ia.getPayload().(*Payload)
	assert.Empty(t, payload.FilesMetadata)
}

func TestGetPayloadFilesMetadataWithoutMainConfig(t *testing.T) {
	tmpDir := t.TempDir()
	systemProbeConfigPath := filepath.Join(tmpDir, "system-probe.yaml")
	require.NoError(t, os.WriteFile(systemProbeConfigPath, []byte("network_config:\n  enabled: true\n"), 0600))

	originalFetchSystemProbeFilename := fetchSystemProbeFilename
	defer func() {
		fetchSystemProbeFilename = originalFetchSystemProbeFilename
	}()
	fetchSystemProbeFilename = func(_ option.Option[sysprobeconfig.Component]) string {
		return systemProbeConfigPath
	}

	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	payload := ia.getPayload().(*Payload)
	require.Len(t, payload.FilesMetadata, 1)
	assert.Contains(t, payload.FilesMetadata, systemProbeConfigPath)
}

func TestGetProvidedConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	payload := ia.getPayload().(*Payload)

	// All configuration level should be in the payload
	assert.Contains(t, payload.Metadata, "full_configuration")
	assert.Contains(t, payload.Metadata, "provided_configuration")
	assert.Contains(t, payload.Metadata, "file_configuration")
	assert.Contains(t, payload.Metadata, "environment_variable_configuration")
	assert.Contains(t, payload.Metadata, "agent_runtime_configuration")
	assert.Contains(t, payload.Metadata, "remote_configuration")
	assert.Contains(t, payload.Metadata, "cli_configuration")
	assert.Contains(t, payload.Metadata, "source_local_configuration")
}

func TestGetProvidedConfigurationOnly(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data := make(agentMetadata)
	ia.getConfigs(data)

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
