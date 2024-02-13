// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	configFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func getTestInventoryPayload(t *testing.T, confOverrides map[string]any, sysprobeConfOverrides map[string]any) *inventoryagent {
	p := newInventoryAgentProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: confOverrides}),
			sysprobeconfigimpl.MockModule(),
			fx.Replace(sysprobeconfigimpl.MockParams{Overrides: sysprobeConfOverrides}),
			fx.Provide(func() serializer.MetricSerializer { return &serializer.MockSerializer{} }),
		),
	)
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
		"dynamic_instrumentation.enabled":                            true,
		"remote_configuration.enabled":                               true,
		"runtime_security_config.enabled":                            true,
		"event_monitoring_config.network.enabled":                    true,
		"runtime_security_config.activity_dump.enabled":              true,
		"runtime_security_config.remote_configuration.enabled":       true,
		"network_config.enabled":                                     true,
		"service_monitoring_config.enable_http_monitoring":           true,
		"service_monitoring_config.tls.native.enabled":               true,
		"service_monitoring_config.enabled":                          true,
		"service_monitoring_config.tls.java.enabled":                 true,
		"service_monitoring_config.enable_http2_monitoring":          true,
		"service_monitoring_config.enable_kafka_monitoring":          true,
		"service_monitoring_config.tls.istio.enabled":                true,
		"service_monitoring_config.enable_http_stats_by_status_code": true,
		"service_monitoring_config.tls.go.enabled":                   true,
		"system_probe_config.enable_tcp_queue_length":                true,
		"system_probe_config.enable_oom_kill":                        true,
		"windows_crash_detection.enabled":                            true,
		"system_probe_config.enable_co_re":                           true,
		"system_probe_config.enable_runtime_compiler":                true,
		"system_probe_config.enable_kernel_header_download":          true,
		"system_probe_config.allow_precompiled_fallback":             true,
		"system_probe_config.telemetry_enabled":                      true,
		"system_probe_config.max_conns_per_message":                  10,
		"system_probe_config.disable_ipv6":                           false,
		"network_config.collect_tcp_v4":                              true,
		"network_config.collect_tcp_v6":                              true,
		"network_config.collect_udp_v4":                              true,
		"network_config.collect_udp_v6":                              true,
		"network_config.enable_protocol_classification":              true,
		"network_config.enable_gateway_lookup":                       true,
		"network_config.enable_root_netns":                           true,
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
		"dd_site":                          "test",

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
		"flavor":                           flavor.GetFlavor(),
		"config_apm_dd_url":                "http://name:********@someintake.example.com/",
		"config_dd_url":                    "http://name:********@someintake.example.com/",
		"config_site":                      "test",
		"config_logs_dd_url":               "http://name:********@someintake.example.com/",
		"config_logs_socks5_proxy_address": "http://name:********@proxy.example.com/",
		"config_process_dd_url":            "http://name:********@someintake.example.com/",
		"config_proxy_http":                "http://name:********@proxy.example.com/",
		"config_proxy_https":               "https://name:********@proxy.example.com/",

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
		"feature_usm_java_tls_enabled":                 true,
		"feature_usm_http2_enabled":                    true,
		"feature_usm_istio_enabled":                    true,
		"feature_usm_http_by_status_code_enabled":      true,
		"feature_usm_go_tls_enabled":                   true,
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
	pkgconfig.Datadog.Set("inventories_max_interval", 10*time.Minute, pkgconfigmodel.SourceAgentRuntime)
	assert.True(t, ia.RefreshTriggered())
}

func TestStatusHeaderProvider(t *testing.T) {
	ret := newInventoryAgentProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: nil}),
			sysprobeconfigimpl.MockModule(),
			fx.Replace(sysprobeconfigimpl.MockParams{Overrides: nil}),
			fx.Provide(func() serializer.MetricSerializer { return &serializer.MockSerializer{} }),
		),
	)

	headerStatusProvider := ret.StatusHeaderProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerStatusProvider.JSON(false, stats)

			assert.NotEmpty(t, stats)
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
		return "", fmt.Errorf("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchSecurityAgentMetadata()

	assert.False(t, ia.data["feature_cspm_enabled"].(bool))
	assert.False(t, ia.data["feature_cspm_host_benchmarks_enabled"].(bool))

	fetchSecurityConfig = func(config pkgconfigmodel.Reader) (string, error) {
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
		return "", fmt.Errorf("some error")
	}

	ia := getTestInventoryPayload(t, nil, nil)
	ia.fetchProcessAgentMetadata()

	assert.False(t, ia.data["feature_process_enabled"].(bool))
	assert.False(t, ia.data["feature_process_language_detection_enabled"].(bool))
	// default to true in the process agent configuration
	assert.True(t, ia.data["feature_processes_container_enabled"].(bool))

	fetchProcessConfig = func(config pkgconfigmodel.Reader) (string, error) {
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

	fetchTraceConfig = func(config pkgconfigmodel.Reader) (string, error) {
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
	defer func() {
		fetchSystemProbeConfig = configFetcher.SystemProbeConfig
	}()
	fetchSystemProbeConfig = func(config pkgconfigmodel.Reader) (string, error) {
		return "", fmt.Errorf("some error")
	}

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
	assert.False(t, ia.data["feature_usm_java_tls_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_http_by_status_code_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_go_tls_enabled"].(bool))
	assert.False(t, ia.data["feature_tcp_queue_length_enabled"].(bool))
	assert.False(t, ia.data["feature_oom_kill_enabled"].(bool))
	assert.False(t, ia.data["feature_windows_crash_detection_enabled"].(bool))
	assert.True(t, ia.data["system_probe_core_enabled"].(bool))
	assert.False(t, ia.data["system_probe_runtime_compilation_enabled"].(bool))
	assert.False(t, ia.data["system_probe_kernel_headers_download_enabled"].(bool))
	assert.True(t, ia.data["system_probe_prebuilt_fallback_enabled"].(bool))
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
			logimpl.MockModule(),
			config.MockModule(),
			sysprobeconfig.NoneModule(),
			fx.Provide(func() serializer.MetricSerializer { return &serializer.MockSerializer{} }),
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
	assert.False(t, ia.data["feature_usm_java_tls_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_http_by_status_code_enabled"].(bool))
	assert.False(t, ia.data["feature_usm_go_tls_enabled"].(bool))
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
  enable_http2_monitoring: true
  enable_http_stats_by_status_code: true

windows_crash_detection:
  enabled: true

system_probe_config:
  enable_tcp_queue_length: true
  enable_oom_kill: true
  enable_co_re: true
  enable_runtime_compiler: true
  enable_kernel_header_download: true
  allow_precompiled_fallback: true
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
	assert.True(t, ia.data["feature_usm_java_tls_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_http2_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_istio_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_http_by_status_code_enabled"].(bool))
	assert.True(t, ia.data["feature_usm_go_tls_enabled"].(bool))
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
