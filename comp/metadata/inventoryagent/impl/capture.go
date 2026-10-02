// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package inventoryagentimpl

import (
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

var _ serializer.InventoryCapture = (*Payload)(nil)

// CaptureInventoryStream identifies only this inventory contract.
func (*Payload) CaptureInventoryStream() telemetrycapture.Stream {
	return telemetrycapture.AgentInventory
}

// captureMetadata borrows only explicitly named scalar fields. It cannot copy
// arbitrary configuration strings, credentials, remote-config target identifiers,
// config_id, or fleet_policies_applied. Feature flags describe capabilities only.
func (p *Payload) captureMetadata() telemetrycapture.AgentInventoryMetadata {
	text := func(key string) string { v, _ := p.Metadata[key].(string); return v }
	flag := func(key string) bool { v, _ := p.Metadata[key].(bool); return v }
	startup, _ := p.Metadata["agent_startup_time_ms"].(int64)
	maxConnections, _ := p.Metadata["system_probe_max_connections_per_message"].(int)
	return telemetrycapture.AgentInventoryMetadata{
		InstallMethodTool:                              text("install_method_tool"),
		InstallMethodToolVersion:                       text("install_method_tool_version"),
		InstallMethodInstallerVersion:                  text("install_method_installer_version"),
		HostnameSource:                                 text("hostname_source"),
		FIPSMode:                                       flag("fips_mode"),
		FeatureAPMEnabled:                              flag("feature_apm_enabled"),
		FeatureContainerImagesEnabled:                  flag("feature_container_images_enabled"),
		FeatureCSMVMContainersEnabled:                  flag("feature_csm_vm_containers_enabled"),
		FeatureCSMVMHostsEnabled:                       flag("feature_csm_vm_hosts_enabled"),
		FeatureCSPMEnabled:                             flag("feature_cspm_enabled"),
		FeatureCSPMHostBenchmarksEnabled:               flag("feature_cspm_host_benchmarks_enabled"),
		FeatureCWSEnabled:                              flag("feature_cws_enabled"),
		FeatureCWSNetworkEnabled:                       flag("feature_cws_network_enabled"),
		FeatureCWSRemoteConfigEnabled:                  flag("feature_cws_remote_config_enabled"),
		FeatureCWSSecurityProfilesEnabled:              flag("feature_cws_security_profiles_enabled"),
		FeatureDiscoveryEnabled:                        flag("feature_discovery_enabled"),
		FeatureDiscoveryServiceMapEnabled:              flag("feature_discovery_service_map_enabled"),
		FeatureDynamicInstrumentationEnabled:           flag("feature_dynamic_instrumentation_enabled"),
		FeatureGPUMonitoringEnabled:                    flag("feature_gpu_monitoring_enabled"),
		FeatureIMDSv2Enabled:                           flag("feature_imdsv2_enabled"),
		FeatureLogsEnabled:                             flag("feature_logs_enabled"),
		FeatureNetworkPathConnectionsMonitoringEnabled: flag("feature_network_path_connections_monitoring_enabled"),
		FeatureNetworkPathRemoteConfigEnabled:          flag("feature_network_path_remote_config_enabled"),
		FeatureOOMKillEnabled:                          flag("feature_oom_kill_enabled"),
		FeatureProcessLanguageDetectionEnabled:         flag("feature_process_language_detection_enabled"),
		FeatureRemoteConfigurationEnabled:              flag("feature_remote_configuration_enabled"),
		FeatureRemoteUpdatesEnabled:                    flag("feature_remote_updates_enabled"),
		FeatureSyntheticsCollectorEnabled:              flag("feature_synthetics_collector_enabled"),
		FeatureTCPQueueLengthEnabled:                   flag("feature_tcp_queue_length_enabled"),
		FeatureTracerouteEnabled:                       flag("feature_traceroute_enabled"),
		FeatureUSMEnabled:                              flag("feature_usm_enabled"),
		FeatureUSMGoTLSEnabled:                         flag("feature_usm_go_tls_enabled"),
		FeatureUSMHTTP2Enabled:                         flag("feature_usm_http2_enabled"),
		FeatureUSMIstioEnabled:                         flag("feature_usm_istio_enabled"),
		FeatureUSMKafkaEnabled:                         flag("feature_usm_kafka_enabled"),
		FeatureUSMPostgresEnabled:                      flag("feature_usm_postgres_enabled"),
		FeatureUSMRedisEnabled:                         flag("feature_usm_redis_enabled"),
		FeatureWindowsCrashDetectionEnabled:            flag("feature_windows_crash_detection_enabled"),
		SystemProbeCoreEnabled:                         flag("system_probe_core_enabled"),
		SystemProbeGatewayLookupEnabled:                flag("system_probe_gateway_lookup_enabled"),
		SystemProbeKernelHeadersDownloadEnabled:        flag("system_probe_kernel_headers_download_enabled"),
		SystemProbeMaxConnectionsPerMessage:            int64(maxConnections),
		SystemProbePrebuiltFallbackEnabled:             flag("system_probe_prebuilt_fallback_enabled"),
		SystemProbeProtocolClassificationEnabled:       flag("system_probe_protocol_classification_enabled"),
		SystemProbeRootNamespaceEnabled:                flag("system_probe_root_namespace_enabled"),
		SystemProbeRuntimeCompilationEnabled:           flag("system_probe_runtime_compilation_enabled"),
		SystemProbeTelemetryEnabled:                    flag("system_probe_telemetry_enabled"),
		SystemProbeTrackTCP4Connections:                flag("system_probe_track_tcp_4_connections"),
		SystemProbeTrackTCP6Connections:                flag("system_probe_track_tcp_6_connections"),
		SystemProbeTrackUDP4Connections:                flag("system_probe_track_udp_4_connections"),
		SystemProbeTrackUDP6Connections:                flag("system_probe_track_udp_6_connections"),

		AgentVersion: text("agent_version"), PackageVersion: text("package_version"),
		Flavor: text("flavor"), InfrastructureMode: text("infrastructure_mode"), AgentStartupTimeMS: startup,
		FeatureProcessEnabled:            flag("feature_process_enabled"),
		FeatureProcessesContainerEnabled: flag("feature_processes_container_enabled"),
		FeatureNetworksEnabled:           flag("feature_networks_enabled"),
		FeatureNetworksHTTPEnabled:       flag("feature_networks_http_enabled"),
		FeatureNetworksHTTPSEnabled:      flag("feature_networks_https_enabled"),
	}
}

// CaptureInventorySize measures the safe projection before owned copying.
func (p *Payload) CaptureInventorySize() int64 {
	a := p.captureMetadata()
	return 256 + telemetrycapture.InventorySize(&telemetrycapture.Inventory{Hostname: p.Hostname, UUID: p.UUID, Agent: &a})
}

// CopyCaptureInventory returns an independent credential-free projection.
func (p *Payload) CopyCaptureInventory() *telemetrycapture.Inventory {
	a := p.captureMetadata()
	return telemetrycapture.CloneInventory(&telemetrycapture.Inventory{Hostname: p.Hostname, UUID: p.UUID, Timestamp: p.Timestamp, Agent: &a})
}
