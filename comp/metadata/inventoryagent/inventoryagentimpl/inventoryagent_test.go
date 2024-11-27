// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"golang.org/x/exp/maps"

	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	configFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher"
	sysprobeConfigFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher/sysprobe"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func getProvides(t *testing.T, confOverrides map[string]any, sysprobeConfOverrides map[string]any) provides {
	return newInventoryAgentProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: confOverrides}),
			sysprobeconfigimpl.MockModule(),
			fx.Replace(sysprobeconfigimpl.MockParams{Overrides: sysprobeConfOverrides}),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			authtokenimpl.Module(),
		),
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

func TestInitDataErrorInstallInfo(t *testing.T) {
	defer func() { installinfoGet = installinfo.Get }()
	installinfoGet = func(config.Reader) (*installinfo.InstallInfo, error) {
		return nil, fmt.Errorf("some error")
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
		"service_monitoring_config.enable_http_monitoring":     true,
		"service_monitoring_config.tls.native.enabled":         true,
		"service_monitoring_config.enabled":                    true,
		"service_monitoring_config.tls.java.enabled":           true,
		"service_monitoring_config.enable_http2_monitoring":    true,
		"service_monitoring_config.enable_kafka_monitoring":    true,
		"service_monitoring_config.enable_postgres_monitoring": true,
		"service_monitoring_config.enable_redis_monitoring":    true,
		"service_monitoring_config.tls.istio.enabled":          true,
		"service_monitoring_config.tls.go.enabled":             true,
		"discovery.enabled":                                    true,
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

		"fips.enabled":                                true,
		"logs_enabled":                                true,
		"compliance_config.enabled":                   true,
		"compliance_config.host_benchmarks.enabled":   true,
		"apm_config.enabled":                          true,
		"ec2_prefer_imdsv2":                           true,
		"process_config.container_collection.enabled": true,
		"remote_configuration.enabled":                true,
		"process_config.process_collection.enabled":   true,
		"container_image.enabled":                     true,
		"sbom.enabled":                                true,
		"sbom.container_image.enabled":                true,
		"sbom.host.enabled":                           true,
	}
	ia := getTestInventoryPayload(t, overrides, sysprobeOverrides)
	ia.refreshMetadata()

	expected := map[string]any{
		"agent_version":                    version.AgentVersion,
		"agent_startup_time_ms":            pkgconfigsetup.StartTime.UnixMilli(),
		"flavor":                           flavor.GetFlavor(),
		"config_apm_dd_url":                "http://name:********@someintake.example.com/",
		"config_dd_url":                    "http://name:********@someintake.example.com/",
		"config_site":                      "test",
		"config_logs_dd_url":               "http://name:********@someintake.example.com/",
		"config_logs_socks5_proxy_address": "http://name:********@proxy.example.com/",
		"config_process_dd_url":            "http://name:********@someintake.example.com/",
		"config_proxy_http":                "http://name:********@proxy.example.com/",
		"config_proxy_https":               "https://name:********@proxy.example.com/",
		"config_eks_fargate":               true,

		"feature_process_language_detection_enabled": true,
		"feature_fips_enabled":                       true,
		"feature_logs_enabled":                       true,
		"feature_cspm_enabled":                       true,
		"feature_cspm_host_benchmarks_enabled":       true,
		"feature_apm_enabled":                        true,
		"feature_imdsv2_enabled":                     true,
		"feature_processes_container_enabled":        true,
		"feature_remote_configuration_enabled":       true,
		"feature_process_enabled":                    true,
		"feature_container_images_enabled":           true,

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
		"feature_usm_enabled":                          true,
		"feature_usm_kafka_enabled":                    true,
		"feature_usm_postgres_enabled":                 true,
		"feature_usm_redis_enabled":                    true,
		"feature_usm_http2_enabled":                    true,
		"feature_usm_istio_enabled":                    true,
		"feature_usm_go_tls_enabled":                   true,
		"feature_discovery_enabled":                    true,
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
	}

	if !kernel.IsIPv6Enabled() {
		expected["system_probe_track_tcp_6_connections"] = false
		expected["system_probe_track_udp_6_connections"] = false
	}

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
	ia := getTestInventoryPayload(t, nil, nil)

	assert.False(t, ia.RefreshTriggered())
	pkgconfigsetup.Datadog().Set("inventories_max_interval", 10*60, pkgconfigmodel.SourceAgentRuntime)
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

			keys := maps.Keys(stats)

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
	fetchSecurityConfig = func(config pkgconfigmodel.Reader) (string, error) {
		// test that the agent config was passed and not the system-probe config.
		assert.False(
			t,
			config.IsSet("system_probe_config.sysprobe_socket"),
			"wrong configuration received for security-agent fetcher",
		)
		assert.True(
			t,
			config.IsSet("hostname"),
			"wrong configuration received for security-agent fetcher",
		)

		return "", fmt.Errorf("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchSecurityAgentMetadata()

	assert.False(t, ia.data["feature_cspm_enabled"].(bool))
	assert.False(t, ia.data["feature_cspm_host_benchmarks_enabled"].(bool))

	fetchSecurityConfig = func(_ pkgconfigmodel.Reader) (string, error) {
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
	defer func(original func(cfg pkgconfigmodel.Reader) (string, error)) {
		fetchProcessConfig = original
	}(fetchProcessConfig)

	fetchProcessConfig = func(config pkgconfigmodel.Reader) (string, error) {
		// test that the agent config was passed and not the system-probe config.
		assert.False(
			t,
			config.IsSet("system_probe_config.sysprobe_socket"),
			"wrong configuration received for process-agent fetcher",
		)
		assert.True(
			t,
			config.IsSet("hostname"),
			"wrong configuration received for security-agent fetcher",
		)

		return "", fmt.Errorf("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchProcessAgentMetadata()

	assert.False(t, ia.data["feature_process_enabled"].(bool))
	assert.False(t, ia.data["feature_process_language_detection_enabled"].(bool))
	// default to true in the process agent configuration
	assert.True(t, ia.data["feature_processes_container_enabled"].(bool))

	fetchProcessConfig = func(_ pkgconfigmodel.Reader) (string, error) {
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
	fetchTraceConfig = func(config pkgconfigmodel.Reader) (string, error) {
		// test that the agent config was passed and not the system-probe config.
		assert.False(
			t,
			config.IsSet("system_probe_config.sysprobe_socket"),
			"wrong configuration received for trace-agent fetcher",
		)
		assert.True(
			t,
			config.IsSet("hostname"),
			"wrong configuration received for security-agent fetcher",
		)

		return "", fmt.Errorf("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchTraceAgentMetadata()

	if runtime.GOARCH == "386" && runtime.GOOS == "windows" {
		assert.False(t, ia.data["feature_apm_enabled"].(bool))
	} else {
		assert.True(t, ia.data["feature_apm_enabled"].(bool))
	}
	assert.Equal(t, "", ia.data["config_apm_dd_url"].(string))

	fetchTraceConfig = func(_ pkgconfigmodel.Reader) (string, error) {
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
	fetchSystemProbeConfig = func(config pkgconfigmodel.Reader) (string, error) {
		// test that the system-probe config was passed and not the agent config
		assert.True(
			t,
			config.IsSet("system_probe_config.sysprobe_socket"),
			"wrong configuration received for system-probe fetcher",
		)
		assert.False(
			t,
			config.IsSet("hostname"),
			"wrong configuration received for security-agent fetcher",
		)

		return "", fmt.Errorf("some error")
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
	assert.False(t, ia.data["feature_networks_http_enabled"].(bool))
	assert.False(t, ia.data["feature_networks_https_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_kafka_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_postgres_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_redis_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_go_tls_enabled"].(bool))
	assert.False(t, ia.data["feature_discovery_enabled"].(bool))
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
	p := newInventoryAgentProvider(
		fxutil.Test[dependencies](
			t,
			fx.Provide(func() log.Component { return logmock.New(t) }),
			config.MockModule(),
			sysprobeconfig.NoneModule(),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			authtokenimpl.Module(),
		),
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
	assert.False(t, ia.data["feature_usm_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_kafka_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_postgres_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_go_tls_enabled"].(bool))
	assert.False(t, ia.data["feature_discovery_enabled"].(bool))
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
	fetchSystemProbeConfig = func(_ pkgconfigmodel.Reader) (string, error) {
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

service_monitoring_config:
  enable_http_monitoring: true
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
  enable_kafka_monitoring: true
  enable_postgres_monitoring: true
  enable_redis_monitoring: true
  enable_http2_monitoring: true
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
	assert.True(t, ia.data["feature_usm_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_kafka_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_postgres_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_redis_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_go_tls_enabled"].(bool))
	assert.True(t, ia.data["feature_discovery_enabled"].(bool))
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
	expected := []string{"provided_configuration", "full_configuration", "file_configuration", "environment_variable_configuration", "agent_runtime_configuration", "fleet_policies_configuration", "remote_configuration", "cli_configuration", "source_local_configuration"}
	sort.Strings(expected)

	assert.Equal(t, expected, keys)
}
