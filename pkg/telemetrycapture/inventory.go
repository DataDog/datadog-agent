// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"encoding/json"
	"strings"
	"unsafe"
)

// Inventory owns native inventory telemetry. Configuration, credentials, and
// remote-management target identifiers are excluded by producer projections.
// Timestamp follows the native inventory envelope's Unix nanoseconds.
type Inventory struct {
	Hostname   string                  `json:"hostname"`
	UUID       string                  `json:"uuid"`
	Timestamp  int64                   `json:"timestamp"`
	Agent      *AgentInventoryMetadata `json:"agent_metadata,omitempty"`
	Host       *HostInventoryMetadata  `json:"host_metadata,omitempty"`
	SystemInfo *HostSystemInfoMetadata `json:"host_system_info_metadata,omitempty"`
}

// HostSystemInfoMetadata carries only the native device hardware fields.
// Replay rewrites the serial number while preserving model identifiers.
type HostSystemInfoMetadata struct {
	Manufacturer string `json:"manufacturer"`
	ModelNumber  string `json:"model_number"`
	SerialNumber string `json:"serial_number"`
	ModelName    string `json:"model_name"`
	ChassisType  string `json:"chassis_type"`
	Identifier   string `json:"identifier"`
}

// AgentInventoryMetadata carries the registration identity and feature flags
// used by device discovery. It cannot contain Agent configuration or secrets.
type AgentInventoryMetadata struct {
	InstallMethodTool                              string `json:"install_method_tool"`
	InstallMethodToolVersion                       string `json:"install_method_tool_version"`
	InstallMethodInstallerVersion                  string `json:"install_method_installer_version"`
	HostnameSource                                 string `json:"hostname_source"`
	FIPSMode                                       bool   `json:"fips_mode"`
	FeatureAPMEnabled                              bool   `json:"feature_apm_enabled"`
	FeatureContainerImagesEnabled                  bool   `json:"feature_container_images_enabled"`
	FeatureCSMVMContainersEnabled                  bool   `json:"feature_csm_vm_containers_enabled"`
	FeatureCSMVMHostsEnabled                       bool   `json:"feature_csm_vm_hosts_enabled"`
	FeatureCSPMEnabled                             bool   `json:"feature_cspm_enabled"`
	FeatureCSPMHostBenchmarksEnabled               bool   `json:"feature_cspm_host_benchmarks_enabled"`
	FeatureCWSEnabled                              bool   `json:"feature_cws_enabled"`
	FeatureCWSNetworkEnabled                       bool   `json:"feature_cws_network_enabled"`
	FeatureCWSRemoteConfigEnabled                  bool   `json:"feature_cws_remote_config_enabled"`
	FeatureCWSSecurityProfilesEnabled              bool   `json:"feature_cws_security_profiles_enabled"`
	FeatureDiscoveryEnabled                        bool   `json:"feature_discovery_enabled"`
	FeatureDiscoveryServiceMapEnabled              bool   `json:"feature_discovery_service_map_enabled"`
	FeatureDynamicInstrumentationEnabled           bool   `json:"feature_dynamic_instrumentation_enabled"`
	FeatureGPUMonitoringEnabled                    bool   `json:"feature_gpu_monitoring_enabled"`
	FeatureIMDSv2Enabled                           bool   `json:"feature_imdsv2_enabled"`
	FeatureLogsEnabled                             bool   `json:"feature_logs_enabled"`
	FeatureNetworkPathConnectionsMonitoringEnabled bool   `json:"feature_network_path_connections_monitoring_enabled"`
	FeatureNetworkPathRemoteConfigEnabled          bool   `json:"feature_network_path_remote_config_enabled"`
	FeatureOOMKillEnabled                          bool   `json:"feature_oom_kill_enabled"`
	FeatureProcessLanguageDetectionEnabled         bool   `json:"feature_process_language_detection_enabled"`
	FeatureRemoteConfigurationEnabled              bool   `json:"feature_remote_configuration_enabled"`
	FeatureRemoteUpdatesEnabled                    bool   `json:"feature_remote_updates_enabled"`
	FeatureSyntheticsCollectorEnabled              bool   `json:"feature_synthetics_collector_enabled"`
	FeatureTCPQueueLengthEnabled                   bool   `json:"feature_tcp_queue_length_enabled"`
	FeatureTracerouteEnabled                       bool   `json:"feature_traceroute_enabled"`
	FeatureUSMEnabled                              bool   `json:"feature_usm_enabled"`
	FeatureUSMGoTLSEnabled                         bool   `json:"feature_usm_go_tls_enabled"`
	FeatureUSMHTTP2Enabled                         bool   `json:"feature_usm_http2_enabled"`
	FeatureUSMIstioEnabled                         bool   `json:"feature_usm_istio_enabled"`
	FeatureUSMKafkaEnabled                         bool   `json:"feature_usm_kafka_enabled"`
	FeatureUSMPostgresEnabled                      bool   `json:"feature_usm_postgres_enabled"`
	FeatureUSMRedisEnabled                         bool   `json:"feature_usm_redis_enabled"`
	FeatureWindowsCrashDetectionEnabled            bool   `json:"feature_windows_crash_detection_enabled"`
	SystemProbeCoreEnabled                         bool   `json:"system_probe_core_enabled"`
	SystemProbeGatewayLookupEnabled                bool   `json:"system_probe_gateway_lookup_enabled"`
	SystemProbeKernelHeadersDownloadEnabled        bool   `json:"system_probe_kernel_headers_download_enabled"`
	SystemProbeMaxConnectionsPerMessage            int64  `json:"system_probe_max_connections_per_message"`
	SystemProbePrebuiltFallbackEnabled             bool   `json:"system_probe_prebuilt_fallback_enabled"`
	SystemProbeProtocolClassificationEnabled       bool   `json:"system_probe_protocol_classification_enabled"`
	SystemProbeRootNamespaceEnabled                bool   `json:"system_probe_root_namespace_enabled"`
	SystemProbeRuntimeCompilationEnabled           bool   `json:"system_probe_runtime_compilation_enabled"`
	SystemProbeTelemetryEnabled                    bool   `json:"system_probe_telemetry_enabled"`
	SystemProbeTrackTCP4Connections                bool   `json:"system_probe_track_tcp_4_connections"`
	SystemProbeTrackTCP6Connections                bool   `json:"system_probe_track_tcp_6_connections"`
	SystemProbeTrackUDP4Connections                bool   `json:"system_probe_track_udp_4_connections"`
	SystemProbeTrackUDP6Connections                bool   `json:"system_probe_track_udp_6_connections"`

	AgentVersion                     string `json:"agent_version"`
	PackageVersion                   string `json:"package_version"`
	Flavor                           string `json:"flavor"`
	InfrastructureMode               string `json:"infrastructure_mode"`
	AgentStartupTimeMS               int64  `json:"agent_startup_time_ms"`
	FeatureProcessEnabled            bool   `json:"feature_process_enabled"`
	FeatureProcessesContainerEnabled bool   `json:"feature_processes_container_enabled"`
	FeatureNetworksEnabled           bool   `json:"feature_networks_enabled"`
	FeatureNetworksHTTPEnabled       bool   `json:"feature_networks_http_enabled"`
	FeatureNetworksHTTPSEnabled      bool   `json:"feature_networks_https_enabled"`
}

// HostInventoryMetadata carries native hardware and OS enrichment. Replay
// rewrites device identifiers and local interface addresses.
type HostInventoryMetadata struct {
	KernelVersion                string `json:"kernel_version"`
	Interfaces                   string `json:"interfaces"`
	CloudProvider                string `json:"cloud_provider"`
	CloudProviderSource          string `json:"cloud_provider_source"`
	CloudProviderAccountID       string `json:"cloud_provider_account_id"`
	CloudProviderHostID          string `json:"cloud_provider_host_id"`
	CanonicalCloudResourceID     string `json:"ccrid"`
	InstanceType                 string `json:"instance-type"`
	HypervisorGuestUUID          string `json:"hypervisor_guest_uuid"`
	DmiProductUUID               string `json:"dmi_product_uuid"`
	DmiBoardAssetTag             string `json:"dmi_board_asset_tag"`
	DmiBoardVendor               string `json:"dmi_board_vendor"`
	LinuxPackageSigningEnabled   bool   `json:"linux_package_signing_enabled"`
	RPMGlobalRepoGPGCheckEnabled bool   `json:"rpm_global_repo_gpg_check_enabled"`

	CPUCores             uint64  `json:"cpu_cores"`
	CPULogicalProcessors uint64  `json:"cpu_logical_processors"`
	CPUVendor            string  `json:"cpu_vendor"`
	CPUModel             string  `json:"cpu_model"`
	CPUModelID           string  `json:"cpu_model_id"`
	CPUFamily            string  `json:"cpu_family"`
	CPUStepping          string  `json:"cpu_stepping"`
	CPUFrequency         float64 `json:"cpu_frequency"`
	CPUCacheSize         uint64  `json:"cpu_cache_size"`
	KernelName           string  `json:"kernel_name"`
	KernelRelease        string  `json:"kernel_release"`
	OS                   string  `json:"os"`
	OSVersion            string  `json:"os_version"`
	CPUArchitecture      string  `json:"cpu_architecture"`
	MemoryTotalKb        uint64  `json:"memory_total_kb"`
	MemorySwapTotalKb    uint64  `json:"memory_swap_total_kb"`
	IPAddress            string  `json:"ip_address"`
	IPv6Address          string  `json:"ipv6_address"`
	MacAddress           string  `json:"mac_address"`
	AgentVersion         string  `json:"agent_version"`
}

// MarshalJSON allows captured inventories to use the normal metadata serializer.
func (i *Inventory) MarshalJSON() ([]byte, error) {
	type plain Inventory
	return json.Marshal((*plain)(i))
}

// InventorySize charges every owned allocation. It can measure a borrowed
// projection before CloneInventory is allowed to allocate its independent copy.
func InventorySize(i *Inventory) int64 {
	if i == nil {
		return 0
	}
	n := int64(unsafe.Sizeof(*i)) + int64(len(i.Hostname)+len(i.UUID))
	if a := i.Agent; a != nil {
		n += int64(unsafe.Sizeof(*a))
		for _, value := range agentInventoryStrings(a) {
			n += int64(len(*value))
		}
	}
	if h := i.Host; h != nil {
		n += int64(unsafe.Sizeof(*h))
		for _, value := range hostInventoryStrings(h) {
			n += int64(len(*value))
		}
	}
	if h := i.SystemInfo; h != nil {
		n += int64(unsafe.Sizeof(*h))
		for _, value := range hostSystemInfoStrings(h) {
			n += int64(len(*value))
		}
	}
	return n
}

// CloneInventory is used only after a producer reserves InventorySize bytes.
func CloneInventory(i *Inventory) *Inventory {
	if i == nil {
		return nil
	}
	out := *i
	out.Hostname, out.UUID = strings.Clone(i.Hostname), strings.Clone(i.UUID)
	if i.Agent != nil {
		a := *i.Agent
		for _, value := range agentInventoryStrings(&a) {
			*value = strings.Clone(*value)
		}
		out.Agent = &a
	}
	if i.Host != nil {
		h := *i.Host
		for _, value := range hostInventoryStrings(&h) {
			*value = strings.Clone(*value)
		}
		out.Host = &h
	}
	if i.SystemInfo != nil {
		h := *i.SystemInfo
		for _, value := range hostSystemInfoStrings(&h) {
			*value = strings.Clone(*value)
		}
		out.SystemInfo = &h
	}
	return &out
}

func hostSystemInfoStrings(h *HostSystemInfoMetadata) [6]*string {
	return [6]*string{&h.Manufacturer, &h.ModelNumber, &h.SerialNumber, &h.ModelName, &h.ChassisType, &h.Identifier}
}

func hostInventoryStrings(h *HostInventoryMetadata) [26]*string {
	return [26]*string{&h.KernelVersion, &h.Interfaces, &h.CloudProvider, &h.CloudProviderSource, &h.CloudProviderAccountID, &h.CloudProviderHostID, &h.CanonicalCloudResourceID, &h.InstanceType, &h.HypervisorGuestUUID, &h.DmiProductUUID, &h.DmiBoardAssetTag, &h.DmiBoardVendor, &h.CPUVendor, &h.CPUModel, &h.CPUModelID, &h.CPUFamily, &h.CPUStepping, &h.KernelName, &h.KernelRelease, &h.OS, &h.OSVersion, &h.CPUArchitecture, &h.IPAddress, &h.IPv6Address, &h.MacAddress, &h.AgentVersion}
}

func agentInventoryStrings(h *AgentInventoryMetadata) [8]*string {
	return [8]*string{&h.InstallMethodTool, &h.InstallMethodToolVersion, &h.InstallMethodInstallerVersion, &h.HostnameSource, &h.AgentVersion, &h.PackageVersion, &h.Flavor, &h.InfrastructureMode}
}
