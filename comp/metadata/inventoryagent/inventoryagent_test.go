// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagent

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/stretchr/testify/assert"
)

func getTestInventoryPayload(t *testing.T, confOverrides map[string]any) *inventoryagent {
	p := newInventoryAgentProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: confOverrides}),
			fx.Provide(func() serializer.MetricSerializer { return &serializer.MockSerializer{} }),
		),
	)
	return p.Comp.(*inventoryagent)
}

func TestSet(t *testing.T) {
	ia := getTestInventoryPayload(t, nil)

	ia.Set("test", 1234)
	assert.Equal(t, 1234, ia.data["test"])
}

func TestGetPayload(t *testing.T) {
	ia := getTestInventoryPayload(t, nil)
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

	ia := getTestInventoryPayload(t, nil)

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
	// TODO: (components) - until system probe configuration is migrated to a component we use the old mock here.
	systemProbeMock := pkgconfig.MockSystemProbe(t)
	systemProbeMock.SetWithoutSource("dynamic_instrumentation.enabled", true)
	systemProbeMock.SetWithoutSource("remote_configuration.enabled", true)
	systemProbeMock.SetWithoutSource("runtime_security_config.enabled", true)
	systemProbeMock.SetWithoutSource("event_monitoring_config.network.enabled", true)
	systemProbeMock.SetWithoutSource("runtime_security_config.activity_dump.enabled", true)
	systemProbeMock.SetWithoutSource("runtime_security_config.remote_configuration.enabled", true)
	systemProbeMock.SetWithoutSource("network_config.enabled", true)
	systemProbeMock.SetWithoutSource("service_monitoring_config.enable_http_monitoring", true)
	systemProbeMock.SetWithoutSource("service_monitoring_config.tls.native.enabled", true)
	systemProbeMock.SetWithoutSource("service_monitoring_config.enabled", true)
	systemProbeMock.SetWithoutSource("data_streams_config.enabled", true)
	systemProbeMock.SetWithoutSource("service_monitoring_config.tls.java.enabled", true)
	systemProbeMock.SetWithoutSource("service_monitoring_config.enable_http2_monitoring", true)
	systemProbeMock.SetWithoutSource("service_monitoring_config.tls.istio.enabled", true)
	systemProbeMock.SetWithoutSource("service_monitoring_config.enable_http_stats_by_status_code", true)
	systemProbeMock.SetWithoutSource("service_monitoring_config.tls.go.enabled", true)
	systemProbeMock.SetWithoutSource("system_probe_config.enable_tcp_queue_length", true)
	systemProbeMock.SetWithoutSource("system_probe_config.enable_oom_kill", true)
	systemProbeMock.SetWithoutSource("windows_crash_detection.enabled", true)
	systemProbeMock.SetWithoutSource("system_probe_config.enable_co_re", true)
	systemProbeMock.SetWithoutSource("system_probe_config.enable_runtime_compiler", true)
	systemProbeMock.SetWithoutSource("system_probe_config.enable_kernel_header_download", true)
	systemProbeMock.SetWithoutSource("system_probe_config.allow_precompiled_fallback", true)
	systemProbeMock.SetWithoutSource("system_probe_config.telemetry_enabled", true)
	systemProbeMock.SetWithoutSource("system_probe_config.max_conns_per_message", 10)
	systemProbeMock.SetWithoutSource("network_config.collect_tcp_v4", true)
	systemProbeMock.SetWithoutSource("network_config.collect_tcp_v6", true)
	systemProbeMock.SetWithoutSource("network_config.collect_udp_v4", true)
	systemProbeMock.SetWithoutSource("network_config.collect_udp_v6", true)
	systemProbeMock.SetWithoutSource("network_config.enable_protocol_classification", true)
	systemProbeMock.SetWithoutSource("network_config.enable_gateway_lookup", true)
	systemProbeMock.SetWithoutSource("network_config.enable_root_netns", true)

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
	ia := getTestInventoryPayload(t, overrides)

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

	for name, value := range expected {
		assert.Equal(t, value, ia.data[name], "value for '%s' is wrong", name)
	}

	assert.ElementsMatch(t, []string{"http://noprox.example.com", "http://name:********@proxy.example.com/"}, ia.data["config_no_proxy"])
}

func TestGet(t *testing.T) {
	ia := getTestInventoryPayload(t, nil)

	ia.Set("test", 1234)

	p := ia.Get()
	assert.Equal(t, 1234, p["test"])

	// verify that the return map is a copy
	p["test"] = 21
	assert.Equal(t, 1234, ia.data["test"])
}

func TestFlareProviderFilename(t *testing.T) {
	ia := getTestInventoryPayload(t, nil)
	assert.Equal(t, "agent.json", ia.FlareFileName)
}

func TestConfigRefresh(t *testing.T) {
	ia := getTestInventoryPayload(t, nil)

	assert.False(t, ia.RefreshTriggered())
	pkgconfig.Datadog.Set("inventories_max_interval", 10*time.Minute, pkgconfigmodel.SourceAgentRuntime)
	assert.True(t, ia.RefreshTriggered())
}
