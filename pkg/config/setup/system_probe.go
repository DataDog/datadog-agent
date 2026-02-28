// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

type transformerFunction func(string) []map[string]string

const (
	defaultConnsMessageBatchSize = 600

	// defaultRuntimeCompilerOutputDir is the default path for output from the system-probe runtime compiler
	defaultRuntimeCompilerOutputDir = "/var/tmp/datadog-agent/system-probe/build"

	// defaultKernelHeadersDownloadDir is the default path for downloading kernel headers for runtime compilation
	defaultKernelHeadersDownloadDir = "/var/tmp/datadog-agent/system-probe/kernel-headers"

	// defaultBTFOutputDir is the default path for extracted BTF
	defaultBTFOutputDir = "/var/tmp/datadog-agent/system-probe/btf"

	// defaultDynamicInstrumentationDebugInfoDir is the default path for debug
	// info for Dynamic Instrumentation. This is the directory where the DWARF
	// data from analyzed binaries is decompressed into during processing.
	defaultDynamicInstrumentationDebugInfoDir = "/tmp/datadog-agent/system-probe/dynamic-instrumentation/decompressed-debug-info"

	// defaultAptConfigDirSuffix is the default path under `/etc` to the apt config directory
	defaultAptConfigDirSuffix = "/apt"

	// defaultYumReposDirSuffix is the default path under `/etc` to the yum repository directory
	defaultYumReposDirSuffix = "/yum.repos.d"

	// defaultZypperReposDirSuffix is the default path under `/etc` to the zypper repository directory
	defaultZypperReposDirSuffix = "/zypp/repos.d"

	defaultOffsetThreshold = 400

	// defaultEnvoyPath is the default path for envoy binary
	defaultEnvoyPath = "/bin/envoy"
)

var (
	// defaultSystemProbeBPFDir is the default path for eBPF programs
	defaultSystemProbeBPFDir = filepath.Join(InstallPath, "embedded/share/system-probe/ebpf")
)

// InitSystemProbeConfig declares all the configuration values normally read from system-probe.yaml.
func InitSystemProbeConfig(cfg pkgconfigmodel.Setup) {
	cfg.BindEnvAndSetDefault("ignore_host_etc", false)
	cfg.BindEnvAndSetDefault("go_core_dump", false)
	cfg.BindEnvAndSetDefault("system_probe_config.disable_thp", true)

	// Auto exit configuration
	cfg.BindEnvAndSetDefault("auto_exit.validation_period", 60)
	cfg.BindEnvAndSetDefault("auto_exit.noprocess.enabled", false)
	cfg.BindEnvAndSetDefault("auto_exit.noprocess.excluded_processes", []string{})

	// statsd
	cfg.BindEnv("bind_host") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("dogstatsd_port", 8125)

	// logging
	cfg.BindEnvAndSetDefault("system_probe_config.log_file", "")
	cfg.BindEnvAndSetDefault("system_probe_config.log_level", "")
	cfg.BindEnvAndSetDefault("log_file", defaultSystemProbeLogFilePath)
	cfg.BindEnvAndSetDefault("log_level", "info", "DD_LOG_LEVEL", "LOG_LEVEL")
	cfg.BindEnvAndSetDefault("syslog_uri", "")
	cfg.BindEnvAndSetDefault("syslog_rfc", false)
	cfg.BindEnvAndSetDefault("log_to_syslog", false)
	cfg.BindEnvAndSetDefault("log_to_console", true)
	cfg.BindEnvAndSetDefault("log_format_json", false)
	cfg.BindEnvAndSetDefault("log_file_max_size", "10Mb")
	cfg.BindEnvAndSetDefault("log_file_max_rolls", 1)
	cfg.BindEnvAndSetDefault("disable_file_logging", false)
	cfg.BindEnvAndSetDefault("log_format_rfc3339", false)

	// secrets backend
	cfg.BindEnvAndSetDefault("secret_backend_command", "")
	cfg.BindEnvAndSetDefault("secret_backend_arguments", []string{})
	cfg.BindEnvAndSetDefault("secret_backend_output_max_size", 1024*1024)
	cfg.BindEnvAndSetDefault("secret_backend_timeout", 30)
	cfg.BindEnvAndSetDefault("secret_backend_command_allow_group_exec_perm", false)
	cfg.BindEnvAndSetDefault("secret_backend_skip_checks", false)

	// settings for system-probe in general
	cfg.BindEnvAndSetDefault("system_probe_config.enabled", false, "DD_SYSTEM_PROBE_ENABLED")
	cfg.BindEnvAndSetDefault("system_probe_config.external", false, "DD_SYSTEM_PROBE_EXTERNAL")
	cfg.SetDefault("system_probe_config.adjusted", false)

	cfg.BindEnvAndSetDefault("system_probe_config.sysprobe_socket", DefaultSystemProbeAddress, "DD_SYSPROBE_SOCKET")
	cfg.BindEnvAndSetDefault("system_probe_config.max_conns_per_message", defaultConnsMessageBatchSize)

	cfg.BindEnvAndSetDefault("system_probe_config.debug_port", 0)
	cfg.BindEnvAndSetDefault("system_probe_config.telemetry_enabled", false, "DD_TELEMETRY_ENABLED")
	cfg.BindEnvAndSetDefault("system_probe_config.telemetry_perf_buffer_emit_per_cpu", false)
	cfg.BindEnvAndSetDefault("system_probe_config.health_port", int64(0), "DD_SYSTEM_PROBE_HEALTH_PORT")

	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.enabled", false, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_ENABLED")
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.site", DefaultSite, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_SITE", "DD_SITE")
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.profile_dd_url", "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_DD_URL", "DD_APM_INTERNAL_PROFILING_DD_URL")
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.api_key", "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_API_KEY", "DD_API_KEY")
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.env", "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_ENV", "DD_ENV")
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.period", 5*time.Minute, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_PERIOD")
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.cpu_duration", 1*time.Minute, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_CPU_DURATION")
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.mutex_profile_fraction", 0)
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.block_profile_rate", 0)
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.enable_goroutine_stacktraces", false)
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.enable_block_profiling", false)
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.enable_mutex_profiling", false)
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.delta_profiles", true)
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.custom_attributes", []string{"module", "rule_id"})
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.unix_socket", "")
	cfg.BindEnvAndSetDefault("system_probe_config.internal_profiling.extra_tags", []string{})

	cfg.BindEnvAndSetDefault("system_probe_config.memory_controller.enabled", false)
	cfg.BindEnvAndSetDefault("system_probe_config.memory_controller.hierarchy", "v1")
	cfg.BindEnvAndSetDefault("system_probe_config.memory_controller.pressure_levels", map[string]string{})
	cfg.BindEnvAndSetDefault("system_probe_config.memory_controller.thresholds", map[string]string{})

	// ebpf general settings
	cfg.BindEnvAndSetDefault("system_probe_config.bpf_debug", false, "DD_SYSTEM_PROBE_CONFIG_BPF_DEBUG", "BPF_DEBUG")
	cfg.BindEnvAndSetDefault("system_probe_config.bpf_dir", defaultSystemProbeBPFDir, "DD_SYSTEM_PROBE_BPF_DIR")
	cfg.BindEnvAndSetDefault("system_probe_config.excluded_linux_versions", []string{})
	cfg.BindEnvAndSetDefault("system_probe_config.enable_tracepoints", false)
	cfg.BindEnvAndSetDefault("system_probe_config.enable_co_re", true, "DD_ENABLE_CO_RE")
	cfg.BindEnvAndSetDefault("system_probe_config.btf_path", "", "DD_SYSTEM_PROBE_BTF_PATH")
	cfg.BindEnvAndSetDefault("system_probe_config.btf_output_dir", defaultBTFOutputDir, "DD_SYSTEM_PROBE_BTF_OUTPUT_DIR")
	cfg.BindEnvAndSetDefault("system_probe_config.remote_config_btf_enabled", false, "DD_SYSTEM_PROBE_REMOTE_CONFIG_BTF_ENABLED")
	cfg.BindEnv("system_probe_config.enable_runtime_compiler", "DD_ENABLE_RUNTIME_COMPILER") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	// deprecated in favor of allow_prebuilt_fallback below
	cfg.BindEnv("system_probe_config.allow_precompiled_fallback", "DD_ALLOW_PRECOMPILED_FALLBACK") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv("system_probe_config.allow_prebuilt_fallback", "DD_ALLOW_PREBUILT_FALLBACK")       //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("system_probe_config.allow_runtime_compiled_fallback", true, "DD_ALLOW_RUNTIME_COMPILED_FALLBACK")
	cfg.BindEnvAndSetDefault("system_probe_config.runtime_compiler_output_dir", defaultRuntimeCompilerOutputDir, "DD_RUNTIME_COMPILER_OUTPUT_DIR")
	cfg.BindEnv("system_probe_config.enable_kernel_header_download", "DD_ENABLE_KERNEL_HEADER_DOWNLOAD") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("system_probe_config.kernel_header_dirs", []string{}, "DD_KERNEL_HEADER_DIRS")
	cfg.BindEnvAndSetDefault("system_probe_config.kernel_header_download_dir", defaultKernelHeadersDownloadDir, "DD_KERNEL_HEADER_DOWNLOAD_DIR")
	cfg.BindEnvAndSetDefault("system_probe_config.apt_config_dir", suffixHostEtc(defaultAptConfigDirSuffix), "DD_APT_CONFIG_DIR")
	cfg.BindEnvAndSetDefault("system_probe_config.yum_repos_dir", suffixHostEtc(defaultYumReposDirSuffix), "DD_YUM_REPOS_DIR")
	cfg.BindEnvAndSetDefault("system_probe_config.zypper_repos_dir", suffixHostEtc(defaultZypperReposDirSuffix), "DD_ZYPPER_REPOS_DIR")
	cfg.BindEnvAndSetDefault("system_probe_config.attach_kprobes_with_kprobe_events_abi", false, "DD_ATTACH_KPROBES_WITH_KPROBE_EVENTS_ABI")

	// Dynamic Instrumentation settings
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.enabled", false, "DD_DYNAMIC_INSTRUMENTATION_ENABLED")
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.offline_mode", false, "DD_DYNAMIC_INSTRUMENTATION_OFFLINE_MODE")
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.probes_file_path", false, "DD_DYNAMIC_INSTRUMENTATION_PROBES_FILE_PATH")
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.snapshot_output_file_path", false, "DD_DYNAMIC_INSTRUMENTATION_SNAPSHOT_FILE_PATH")
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.diagnostics_output_file_path", false, "DD_DYNAMIC_INSTRUMENTATION_DIAGNOSTICS_FILE_PATH")
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.symdb_upload_enabled", true, "DD_SYMBOL_DATABASE_UPLOAD_ENABLED")
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.enabled", true)
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.dir", defaultDynamicInstrumentationDebugInfoDir)
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.max_total_bytes", int64(2<<30 /* 2GiB */))
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.required_disk_space_bytes", int64(512<<20 /* 512MiB */))
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.required_disk_space_percent", float64(0.0))
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.circuit_breaker.interval", 1*time.Second)
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.circuit_breaker.per_probe_cpu_limit", 0.1)
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.circuit_breaker.all_probes_cpu_limit", 0.5)
	cfg.BindEnvAndSetDefault("dynamic_instrumentation.circuit_breaker.interrupt_overhead", 2*time.Microsecond)

	// network_tracer settings
	// we cannot use BindEnvAndSetDefault for network_config.enabled because we need to know if it was manually set.
	cfg.BindEnv("network_config.enabled", "DD_SYSTEM_PROBE_NETWORK_ENABLED") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv' //nolint:errcheck
	cfg.BindEnvAndSetDefault("system_probe_config.disable_tcp", false, "DD_DISABLE_TCP_TRACING")
	cfg.BindEnvAndSetDefault("system_probe_config.disable_udp", false, "DD_DISABLE_UDP_TRACING")
	cfg.BindEnvAndSetDefault("system_probe_config.disable_ipv6", false, "DD_DISABLE_IPV6_TRACING")

	cfg.SetDefault("network_config.collect_tcp_v4", true)
	cfg.SetDefault("network_config.collect_tcp_v6", true)
	cfg.SetDefault("network_config.collect_udp_v4", true)
	cfg.SetDefault("network_config.collect_udp_v6", true)

	cfg.BindEnvAndSetDefault("system_probe_config.offset_guess_threshold", int64(defaultOffsetThreshold))

	cfg.BindEnvAndSetDefault("system_probe_config.max_tracked_connections", 65536)
	cfg.BindEnv("system_probe_config.max_closed_connections_buffered")   //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv("network_config.max_failed_connections_buffered")        //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv("system_probe_config.closed_connection_flush_threshold") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv("network_config.closed_connection_flush_threshold")      //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv("system_probe_config.closed_channel_size")               //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv("network_config.closed_channel_size")                    //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("network_config.closed_buffer_wakeup_count", 4)
	cfg.BindEnvAndSetDefault("system_probe_config.max_connection_state_buffered", 75000)

	cfg.BindEnvAndSetDefault("system_probe_config.disable_dns_inspection", false, "DD_DISABLE_DNS_INSPECTION")
	cfg.BindEnvAndSetDefault("system_probe_config.collect_dns_stats", true, "DD_COLLECT_DNS_STATS")
	cfg.BindEnvAndSetDefault("system_probe_config.collect_local_dns", false, "DD_COLLECT_LOCAL_DNS")
	cfg.BindEnvAndSetDefault("system_probe_config.collect_dns_domains", true, "DD_COLLECT_DNS_DOMAINS")
	cfg.BindEnvAndSetDefault("system_probe_config.max_dns_stats", 20000)
	cfg.BindEnvAndSetDefault("system_probe_config.dns_timeout_in_s", 15)
	cfg.BindEnvAndSetDefault("network_config.dns_monitoring_ports", []int{53})

	cfg.BindEnvAndSetDefault("system_probe_config.enable_conntrack", true)
	cfg.BindEnvAndSetDefault("system_probe_config.conntrack_max_state_size", 65536*2)
	cfg.BindEnvAndSetDefault("system_probe_config.conntrack_rate_limit", 500)
	cfg.BindEnvAndSetDefault("system_probe_config.enable_conntrack_all_namespaces", true, "DD_SYSTEM_PROBE_ENABLE_CONNTRACK_ALL_NAMESPACES")
	cfg.BindEnvAndSetDefault("network_config.enable_protocol_classification", true, "DD_ENABLE_PROTOCOL_CLASSIFICATION")
	cfg.BindEnvAndSetDefault("network_config.enable_ringbuffers", true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_RINGBUFFERS")
	cfg.BindEnvAndSetDefault("network_config.enable_custom_batching", false, "DD_SYSTEM_PROBE_NETWORK_ENABLE_CUSTOM_BATCHING")
	cfg.BindEnvAndSetDefault("network_config.enable_tcp_failed_connections", true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_FAILED_CONNS")
	cfg.BindEnvAndSetDefault("network_config.ignore_conntrack_init_failure", false, "DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE")
	cfg.BindEnvAndSetDefault("network_config.conntrack_init_timeout", 10*time.Second)
	cfg.BindEnvAndSetDefault("network_config.allow_netlink_conntracker_fallback", true)
	cfg.BindEnvAndSetDefault("network_config.enable_ebpf_conntracker", true)
	cfg.BindEnvAndSetDefault("network_config.enable_cilium_lb_conntracker", true)

	cfg.BindEnvAndSetDefault("system_probe_config.source_excludes", map[string][]string{})
	cfg.BindEnvAndSetDefault("system_probe_config.dest_excludes", map[string][]string{})

	cfg.BindEnvAndSetDefault("system_probe_config.language_detection.enabled", false)

	cfg.BindEnvAndSetDefault("system_probe_config.process_service_inference.use_improved_algorithm", false)

	// For backward compatibility
	cfg.BindEnv("service_monitoring_config.process_service_inference.enabled", "DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_ENABLED") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("system_probe_config.process_service_inference.enabled", runtime.GOOS == "windows")

	// For backward compatibility
	cfg.BindEnv("service_monitoring_config.process_service_inference.use_windows_service_name", "DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_USE_WINDOWS_SERVICE_NAME") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	// default on windows is now enabled; default on linux is still disabled
	cfg.BindEnvAndSetDefault("system_probe_config.process_service_inference.use_windows_service_name", true)

	// network_config namespace only
	cfg.BindEnvAndSetDefault("network_config.enable_gateway_lookup", true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP")

	cfg.BindEnvAndSetDefault("system_probe_config.expected_tags_duration", 30*time.Minute, "DD_SYSTEM_PROBE_EXPECTED_TAGS_DURATION")

	// list of DNS query types to be recorded
	cfg.BindEnvAndSetDefault("network_config.dns_recorded_query_types", []string{})
	// (temporary) enable submitting DNS stats by query type.
	cfg.BindEnvAndSetDefault("network_config.enable_dns_by_querytype", false)
	// connection aggregation with port rollups
	cfg.BindEnvAndSetDefault("network_config.enable_connection_rollup", false)

	cfg.BindEnvAndSetDefault("network_config.enable_ebpfless", false, "DD_ENABLE_EBPFLESS", "DD_NETWORK_CONFIG_ENABLE_EBPFLESS")

	cfg.BindEnvAndSetDefault("network_config.enable_fentry", false)

	// TLS cert collection
	cfg.BindEnvAndSetDefault("network_config.enable_cert_collection", false)
	cfg.BindEnvAndSetDefault("network_config.cert_collection_map_cleaner_interval", 30*time.Second)

	// windows config
	cfg.BindEnvAndSetDefault("system_probe_config.windows.enable_monotonic_count", false)

	// oom_kill module
	cfg.BindEnvAndSetDefault("system_probe_config.enable_oom_kill", false)

	// tcp_queue_length module
	cfg.BindEnvAndSetDefault("system_probe_config.enable_tcp_queue_length", false)
	// process module
	// nested within system_probe_config to not conflict with process-agent's process_config
	cfg.BindEnvAndSetDefault("system_probe_config.process_config.enabled", false, "DD_SYSTEM_PROBE_PROCESS_ENABLED")
	// ebpf module
	cfg.BindEnvAndSetDefault("ebpf_check.enabled", false)
	cfg.BindEnvAndSetDefault("ebpf_check.kernel_bpf_stats", false)
	// noisy neighbor module
	cfg.BindEnvAndSetDefault("noisy_neighbor.enabled", false)

	// settings for the entry count of the ebpfcheck
	// control the size of the buffers used for the batch lookups of the ebpf maps
	cfg.BindEnvAndSetDefault("ebpf_check.entry_count.max_keys_buffer_size_bytes", 512*1024)
	cfg.BindEnvAndSetDefault("ebpf_check.entry_count.max_values_buffer_size_bytes", 1024*1024)
	// How many times we can restart the entry count of a map before we give up if we get an iteration restart
	// due to the map changing while we look it up
	cfg.BindEnvAndSetDefault("ebpf_check.entry_count.max_restarts", 3)
	// How many entries we should keep track of in the entry count map to detect restarts in the
	// single-item iteration
	cfg.BindEnvAndSetDefault("ebpf_check.entry_count.entries_for_iteration_restart_detection", 100)

	// event monitoring
	cfg.BindEnvAndSetDefault("event_monitoring_config.network_process.enabled", true, "DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED")
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.enable_all_probes", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.enable_kernel_filters", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.enable_approvers", false)  // will be set to true by sanitize() if enable_kernel_filters is true
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.enable_discarders", false) // will be set to true by sanitize() if enable_kernel_filters is true
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.flush_discarder_window", 3)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.pid_cache_size", 10000)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.dns_resolution.cache_size", 1024)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.dns_resolution.enabled", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.events_stats.tags_cardinality", "high")
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.custom_sensitive_words", []string{})
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.custom_sensitive_regexps", []string{})
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.erpc_dentry_resolution_enabled", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.map_dentry_resolution_enabled", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.dentry_cache_size", 8000)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.lazy_interface_prefixes", []string{})
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.classifier_priority", 10)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.classifier_handle", 0)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.flow_monitor.enabled", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.flow_monitor.sk_storage.enabled", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.flow_monitor.period", "10s")
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.raw_classifier_handle", 0)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.event_stream.use_ring_buffer", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.event_stream.use_fentry", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.event_stream.use_kprobe_fallback", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.event_stream.buffer_size", 0)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.event_stream.kretprobe_max_active", 512)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.envs_with_value", []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "PATH", "HISTSIZE", "HISTFILESIZE", "GLIBC_TUNABLES", "SSH_CLIENT"})
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.runtime_compilation.enabled", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.enabled", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.ingress.enabled", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.raw_packet.enabled", true)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.raw_packet.limiter_rate", 10)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.raw_packet.filter", "no_pid_tcp_syn")
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.private_ip_ranges", DefaultPrivateIPCIDRs)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.network.extra_private_ip_ranges", []string{})
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.events_stats.polling_interval", 20)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.syscalls_monitor.enabled", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.span_tracking.enabled", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.span_tracking.cache_size", 4096)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.capabilities_monitoring.enabled", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.capabilities_monitoring.period", "5s")
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.snapshot_using_listmount", false)
	eventMonitorBindEnvAndSetDefault(cfg, "event_monitoring_config.event_processing_time.enabled", true)
	cfg.BindEnvAndSetDefault("event_monitoring_config.env_vars_resolution.enabled", true)

	// process event monitoring data limits for network tracer
	eventMonitorBindEnv(cfg, "event_monitoring_config.network_process.max_processes_tracked")

	cfg.BindEnvAndSetDefault("event_monitoring_config.network_process.container_store.enabled", true)
	cfg.BindEnvAndSetDefault("event_monitoring_config.network_process.container_store.max_containers_tracked", 1024)

	cfg.BindEnvAndSetDefault("compliance_config.database_benchmarks.enabled", false)

	// enable/disable use of root net namespace
	cfg.BindEnvAndSetDefault("network_config.enable_root_netns", true)

	// Windows crash detection
	cfg.BindEnvAndSetDefault("windows_crash_detection.enabled", false)

	// Ping
	cfg.BindEnvAndSetDefault("ping.enabled", false)

	// Traceroute
	cfg.BindEnvAndSetDefault("traceroute.enabled", false)

	// CCM config
	cfg.BindEnvAndSetDefault("ccm_network_config.enabled", false)

	// Discovery config
	cfg.BindEnv("discovery.enabled") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("discovery.use_sd_agent", false)
	cfg.BindEnvAndSetDefault("discovery.cpu_usage_update_delay", "60s")
	cfg.BindEnvAndSetDefault("discovery.ignored_command_names", []string{"chronyd", "cilium-agent", "containerd", "dhclient", "dockerd", "kubelet", "livenessprobe", "local-volume-pr", "sshd", "systemd"})
	cfg.BindEnvAndSetDefault("discovery.service_collection_interval", "60s")

	// Privileged Logs config
	cfg.BindEnvAndSetDefault("privileged_logs.enabled", false)

	// Fleet policies
	cfg.BindEnv("fleet_policies_dir") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

	// GPU monitoring
	cfg.BindEnvAndSetDefault("gpu_monitoring.enabled", false)
	cfg.BindEnv("gpu_monitoring.nvml_lib_path") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("gpu_monitoring.process_scan_interval_seconds", 5)
	cfg.BindEnvAndSetDefault("gpu_monitoring.initial_process_sync", true)
	cfg.BindEnvAndSetDefault("gpu_monitoring.configure_cgroup_perms", false)
	cfg.BindEnvAndSetDefault("gpu_monitoring.enable_fatbin_parsing", false)
	cfg.BindEnvAndSetDefault("gpu_monitoring.fatbin_request_queue_size", 100)
	cfg.BindEnvAndSetDefault("gpu_monitoring.ring_buffer_pages_per_device", 32) // 32 pages = 128KB by default per device
	cfg.BindEnvAndSetDefault("gpu_monitoring.ringbuffer_wakeup_size", 3000)     // 3000 bytes is about ~10-20 events depending on the specific type
	cfg.BindEnvAndSetDefault("gpu_monitoring.attacher_detailed_logs", false)
	cfg.BindEnvAndSetDefault("gpu_monitoring.ringbuffer_flush_interval", 1*time.Second)
	cfg.BindEnvAndSetDefault("gpu_monitoring.device_cache_refresh_interval", 5*time.Second)
	cfg.BindEnvAndSetDefault("gpu_monitoring.cgroup_reapply_interval", 30*time.Second)
	cfg.BindEnvAndSetDefault("gpu_monitoring.cgroup_reapply_infinitely", false)

	// Windows Injector telemetry, enabled by default
	cfg.BindEnvAndSetDefault("injector.enable_telemetry", true)

	// gpu - stream config
	cfg.BindEnvAndSetDefault("gpu_monitoring.streams.max_kernel_launches", 1000)
	cfg.BindEnvAndSetDefault("gpu_monitoring.streams.max_mem_alloc_events", 1000)
	cfg.BindEnvAndSetDefault("gpu_monitoring.streams.max_pending_kernel_spans", 1000)
	cfg.BindEnvAndSetDefault("gpu_monitoring.streams.max_pending_memory_spans", 1000)
	cfg.BindEnvAndSetDefault("gpu_monitoring.streams.max_active", 100)
	cfg.BindEnvAndSetDefault("gpu_monitoring.streams.timeout_seconds", 30) // 30 seconds by default, includes two checks at the default interval of 15 seconds

	initCWSSystemProbeConfig(cfg)
	initUSMSystemProbeConfig(cfg)

	cfg.BindEnvAndSetDefault("network_config.direct_send", false)
}

func suffixHostEtc(suffix string) string {
	if value, _ := os.LookupEnv("HOST_ETC"); value != "" {
		return path.Join(value, suffix)
	}
	return path.Join("/etc", suffix)
}

// eventMonitorBindEnvAndSetDefault is a helper function that generates both "DD_RUNTIME_SECURITY_CONFIG_" and "DD_EVENT_MONITORING_CONFIG_"
// prefixes from a key. We need this helper function because the standard BindEnvAndSetDefault can only generate one prefix, but we want to
// support both for backwards compatibility.
func eventMonitorBindEnvAndSetDefault(config pkgconfigmodel.Setup, key string, val interface{}) {
	// Uppercase, replace "." with "_" and add "DD_" prefix to key so that we follow the same environment
	// variable convention as the core agent.
	emConfigKey := "DD_" + strings.ReplaceAll(strings.ToUpper(key), ".", "_")
	runtimeSecKey := strings.Replace(emConfigKey, "EVENT_MONITORING_CONFIG", "RUNTIME_SECURITY_CONFIG", 1)

	envs := []string{emConfigKey, runtimeSecKey}
	config.BindEnvAndSetDefault(key, val, envs...)
}

// eventMonitorBindEnv is the same as eventMonitorBindEnvAndSetDefault, but without setting a default.
func eventMonitorBindEnv(config pkgconfigmodel.Setup, key string) {
	emConfigKey := "DD_" + strings.ReplaceAll(strings.ToUpper(key), ".", "_")
	runtimeSecKey := strings.Replace(emConfigKey, "EVENT_MONITORING_CONFIG", "RUNTIME_SECURITY_CONFIG", 1)

	config.BindEnv(key, emConfigKey, runtimeSecKey) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
}

// DefaultPrivateIPCIDRs is a list of private IP CIDRs that are used to determine if an IP is private or not.
var DefaultPrivateIPCIDRs = []string{
	// IETF RPC 1918
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	// IETF RFC 5735
	"0.0.0.0/8",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	// IETF RFC 6598
	"100.64.0.0/10",
	// IETF RFC 4193
	"fc00::/7",
	// IPv6 loopback address
	"::1/128",
}
