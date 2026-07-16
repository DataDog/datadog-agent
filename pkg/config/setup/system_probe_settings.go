// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
)

func initMainSystemProbeConfig(config pkgconfigmodel.Setup) {
	config.BindEnvAndSetDefault("ignore_host_etc", false)
	config.BindEnvAndSetDefault("go_core_dump", false)
	config.BindEnvAndSetDefault("system_probe_config.disable_thp", true)

	// Auto exit configuration
	config.BindEnvAndSetDefault("auto_exit.validation_period", 60)
	config.BindEnvAndSetDefault("auto_exit.noprocess.enabled", false)
	config.BindEnvAndSetDefault("auto_exit.noprocess.excluded_processes", []string{})

	// statsd
	config.BindEnvAndSetDefault("bind_host", "localhost")
	config.BindEnvAndSetDefault("dogstatsd_port", 8125)

	// logging
	config.BindEnvAndSetDefault("system_probe_config.log_file", "")
	config.BindEnvAndSetDefault("system_probe_config.log_level", "")
	config.BindEnvAndSetDefault("log_file", "${log_path}/system-probe.log")
	config.BindEnvAndSetDefault("log_level", "info", "DD_LOG_LEVEL", "LOG_LEVEL")
	config.BindEnvAndSetDefault("syslog_uri", "")
	config.BindEnvAndSetDefault("syslog_rfc", false)
	config.BindEnvAndSetDefault("log_to_syslog", false)
	config.BindEnvAndSetDefault("log_to_console", true)
	config.BindEnvAndSetDefault("log_format_json", false)
	config.BindEnvAndSetDefault("log_file_max_size", "10Mb")
	config.BindEnvAndSetDefault("log_file_max_rolls", 1)
	config.BindEnvAndSetDefault("disable_file_logging", false)
	config.BindEnvAndSetDefault("log_format_rfc3339", false)

	// secrets backend
	config.BindEnvAndSetDefault("secret_backend_command", "")
	config.BindEnvAndSetDefault("secret_backend_arguments", []string{})
	config.BindEnvAndSetDefault("secret_backend_output_max_size", 1024*1024)
	config.BindEnvAndSetDefault("secret_backend_timeout", 30)
	config.BindEnvAndSetDefault("secret_backend_command_allow_group_exec_perm", false)
	config.BindEnvAndSetDefault("secret_backend_skip_checks", false)

	// settings for system-probe in general
	config.BindEnvAndSetDefault("system_probe_config.enabled", false, "DD_SYSTEM_PROBE_ENABLED")
	config.BindEnvAndSetDefault("system_probe_config.external", false, "DD_SYSTEM_PROBE_EXTERNAL")
	config.SetDefault("system_probe_config.adjusted", false)

	config.BindEnvAndSetDefault("system_probe_config.sysprobe_socket", GetPlatformDefault(map[string]interface{}{
		"linux":   "${run_path}/sysprobe.sock",
		"darwin":  "${run_path}/sysprobe.sock",
		"aix":     "${run_path}/sysprobe.sock",
		"windows": `\\.\pipe\dd_system_probe`,
	}), "DD_SYSPROBE_SOCKET")
	config.BindEnvAndSetDefault("system_probe_config.max_conns_per_message", defaultConnsMessageBatchSize)

	config.BindEnvAndSetDefault("system_probe_config.debug_port", 0)
	config.BindEnvAndSetDefault("system_probe_config.telemetry_enabled", false, "DD_TELEMETRY_ENABLED")
	config.BindEnvAndSetDefault("system_probe_config.telemetry_perf_buffer_emit_per_cpu", false)
	config.BindEnvAndSetDefault("system_probe_config.health_port", int64(0), "DD_SYSTEM_PROBE_HEALTH_PORT")

	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.enabled", false, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_ENABLED")
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.site", DefaultSite, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_SITE", "DD_SITE")
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.profile_dd_url", "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_DD_URL", "DD_APM_INTERNAL_PROFILING_DD_URL")
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.api_key", "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_API_KEY", "DD_API_KEY")
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.env", "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_ENV", "DD_ENV")
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.period", 5*time.Minute, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_PERIOD")
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.cpu_duration", 1*time.Minute, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_CPU_DURATION")
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.mutex_profile_fraction", 0)
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.block_profile_rate", 0)
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.enable_goroutine_stacktraces", false)
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.enable_block_profiling", false)
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.enable_mutex_profiling", false)
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.delta_profiles", true)
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.custom_attributes", []string{"module", "rule_id"})
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.unix_socket", "")
	config.BindEnvAndSetDefault("system_probe_config.internal_profiling.extra_tags", []string{})

	config.BindEnvAndSetDefault("system_probe_config.memory_controller.enabled", false)
	config.BindEnvAndSetDefault("system_probe_config.memory_controller.hierarchy", "v1")
	config.BindEnvAndSetDefault("system_probe_config.memory_controller.pressure_levels", map[string]string{})
	config.BindEnvAndSetDefault("system_probe_config.memory_controller.thresholds", map[string]string{})

	// ebpf general settings
	config.BindEnvAndSetDefault("system_probe_config.bpf_debug", false, "DD_SYSTEM_PROBE_CONFIG_BPF_DEBUG", "BPF_DEBUG")
	config.BindEnvAndSetDefault("system_probe_config.bpf_dir", "${install_path}/embedded/share/system-probe/ebpf", "DD_SYSTEM_PROBE_BPF_DIR")
	config.BindEnvAndSetDefault("system_probe_config.excluded_linux_versions", []string{})
	config.BindEnvAndSetDefault("system_probe_config.enable_tracepoints", false)
	config.BindEnvAndSetDefault("system_probe_config.enable_co_re", true, "DD_ENABLE_CO_RE")
	config.BindEnvAndSetDefault("system_probe_config.btf_path", "", "DD_SYSTEM_PROBE_BTF_PATH")
	config.BindEnvAndSetDefault("system_probe_config.btf_output_dir", defaultBTFOutputDir, "DD_SYSTEM_PROBE_BTF_OUTPUT_DIR")
	config.BindEnvAndSetDefault("system_probe_config.remote_config_btf_enabled", true, "DD_SYSTEM_PROBE_REMOTE_CONFIG_BTF_ENABLED")
	config.BindEnvAndSetDefault("system_probe_config.enable_runtime_compiler", false, "DD_ENABLE_RUNTIME_COMPILER")
	// deprecated in favor of allow_prebuilt_fallback below
	config.BindEnvAndSetDefault("system_probe_config.allow_precompiled_fallback", false, "DD_ALLOW_PRECOMPILED_FALLBACK")
	config.BindEnvAndSetDefault("system_probe_config.allow_prebuilt_fallback", false, "DD_ALLOW_PREBUILT_FALLBACK")
	config.BindEnvAndSetDefault("system_probe_config.allow_runtime_compiled_fallback", true, "DD_ALLOW_RUNTIME_COMPILED_FALLBACK")
	config.BindEnvAndSetDefault("system_probe_config.runtime_compiler_output_dir", defaultRuntimeCompilerOutputDir, "DD_RUNTIME_COMPILER_OUTPUT_DIR")
	config.BindEnvAndSetDefault("system_probe_config.enable_kernel_header_download", false, "DD_ENABLE_KERNEL_HEADER_DOWNLOAD")
	config.BindEnvAndSetDefault("system_probe_config.kernel_header_dirs", []string{}, "DD_KERNEL_HEADER_DIRS")
	config.BindEnvAndSetDefault("system_probe_config.kernel_header_download_dir", defaultKernelHeadersDownloadDir, "DD_KERNEL_HEADER_DOWNLOAD_DIR")
	config.BindEnvAndSetDefault("system_probe_config.apt_config_dir", suffixHostEtc(defaultAptConfigDirSuffix), "DD_APT_CONFIG_DIR")
	config.BindEnvAndSetDefault("system_probe_config.yum_repos_dir", suffixHostEtc(defaultYumReposDirSuffix), "DD_YUM_REPOS_DIR")
	config.BindEnvAndSetDefault("system_probe_config.zypper_repos_dir", suffixHostEtc(defaultZypperReposDirSuffix), "DD_ZYPPER_REPOS_DIR")
	config.BindEnvAndSetDefault("system_probe_config.attach_kprobes_with_kprobe_events_abi", false, "DD_ATTACH_KPROBES_WITH_KPROBE_EVENTS_ABI")

	// Dynamic Instrumentation settings
	config.BindEnvAndSetDefault("dynamic_instrumentation.enabled", false, "DD_DYNAMIC_INSTRUMENTATION_ENABLED")
	config.BindEnvAndSetDefault("dynamic_instrumentation.offline_mode", false, "DD_DYNAMIC_INSTRUMENTATION_OFFLINE_MODE")
	config.BindEnvAndSetDefault("dynamic_instrumentation.probes_file_path", false, "DD_DYNAMIC_INSTRUMENTATION_PROBES_FILE_PATH")
	config.BindEnvAndSetDefault("dynamic_instrumentation.snapshot_output_file_path", false, "DD_DYNAMIC_INSTRUMENTATION_SNAPSHOT_FILE_PATH")
	config.BindEnvAndSetDefault("dynamic_instrumentation.diagnostics_output_file_path", false, "DD_DYNAMIC_INSTRUMENTATION_DIAGNOSTICS_FILE_PATH")
	config.BindEnvAndSetDefault("dynamic_instrumentation.symdb_upload_enabled", true, "DD_SYMBOL_DATABASE_UPLOAD_ENABLED")
	config.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.enabled", true)
	config.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.dir", defaultDynamicInstrumentationDebugInfoDir)
	config.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.max_total_bytes", int64(2<<30 /* 2GiB */))
	config.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.required_disk_space_bytes", int64(512<<20 /* 512MiB */))
	config.BindEnvAndSetDefault("dynamic_instrumentation.debug_info_disk_cache.required_disk_space_percent", float64(0.0))
	config.BindEnvAndSetDefault("dynamic_instrumentation.circuit_breaker.interval", 1*time.Second)
	config.BindEnvAndSetDefault("dynamic_instrumentation.circuit_breaker.per_probe_cpu_limit", float64(0.1))
	config.BindEnvAndSetDefault("dynamic_instrumentation.circuit_breaker.all_probes_cpu_limit", float64(0.5))
	config.BindEnvAndSetDefault("dynamic_instrumentation.circuit_breaker.interrupt_overhead", 2*time.Microsecond)

	// network_tracer settings
	config.BindEnvAndSetDefault("network_config.enabled", false, "DD_SYSTEM_PROBE_NETWORK_ENABLED")
	config.BindEnvAndSetDefault("system_probe_config.disable_tcp", false, "DD_DISABLE_TCP_TRACING")
	config.BindEnvAndSetDefault("system_probe_config.disable_udp", false, "DD_DISABLE_UDP_TRACING")
	config.BindEnvAndSetDefault("system_probe_config.disable_ipv6", false, "DD_DISABLE_IPV6_TRACING")

	config.SetDefault("network_config.collect_tcp_v4", true)
	config.SetDefault("network_config.collect_tcp_v6", true)
	config.SetDefault("network_config.collect_udp_v4", true)
	config.SetDefault("network_config.collect_udp_v6", true)

	config.BindEnvAndSetDefault("system_probe_config.offset_guess_threshold", int64(defaultOffsetThreshold))

	config.BindEnvAndSetDefault("system_probe_config.max_tracked_connections", int64(65536))
	config.BindEnvAndSetDefault("system_probe_config.max_closed_connections_buffered", int64(0))
	config.BindEnvAndSetDefault("network_config.max_failed_connections_buffered", int64(0))
	config.BindEnvAndSetDefault("system_probe_config.closed_connection_flush_threshold", 0)
	config.BindEnvAndSetDefault("network_config.closed_connection_flush_threshold", 0)
	config.BindEnvAndSetDefault("system_probe_config.closed_channel_size", 0)
	config.BindEnvAndSetDefault("network_config.closed_channel_size", 500)
	config.BindEnvAndSetDefault("network_config.closed_buffer_wakeup_count", 4)
	config.BindEnvAndSetDefault("system_probe_config.max_connection_state_buffered", 75000)

	config.BindEnvAndSetDefault("system_probe_config.disable_dns_inspection", false, "DD_DISABLE_DNS_INSPECTION")
	config.BindEnvAndSetDefault("system_probe_config.collect_dns_stats", true, "DD_COLLECT_DNS_STATS")
	config.BindEnvAndSetDefault("system_probe_config.collect_local_dns", false, "DD_COLLECT_LOCAL_DNS")
	config.BindEnvAndSetDefault("system_probe_config.collect_dns_domains", true, "DD_COLLECT_DNS_DOMAINS")
	config.BindEnvAndSetDefault("system_probe_config.max_dns_stats", 20000)
	config.BindEnvAndSetDefault("system_probe_config.dns_timeout_in_s", 15)
	config.BindEnvAndSetDefault("network_config.dns_monitoring_ports", []int{53})

	config.BindEnvAndSetDefault("system_probe_config.enable_conntrack", true)
	config.BindEnvAndSetDefault("system_probe_config.conntrack_max_state_size", 65536*2)
	config.BindEnvAndSetDefault("system_probe_config.conntrack_rate_limit", 500)
	config.BindEnvAndSetDefault("system_probe_config.enable_conntrack_all_namespaces", true, "DD_SYSTEM_PROBE_ENABLE_CONNTRACK_ALL_NAMESPACES")
	config.BindEnvAndSetDefault("network_config.enable_protocol_classification", true, "DD_ENABLE_PROTOCOL_CLASSIFICATION")
	config.BindEnvAndSetDefault("network_config.enable_ringbuffers", true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_RINGBUFFERS")
	config.BindEnvAndSetDefault("network_config.enable_tcp_failed_connections", true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_FAILED_CONNS")
	config.BindEnvAndSetDefault("network_config.ignore_conntrack_init_failure", false, "DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE")
	config.BindEnvAndSetDefault("network_config.conntrack_init_timeout", 10*time.Second)
	config.BindEnvAndSetDefault("network_config.allow_netlink_conntracker_fallback", true)
	config.BindEnvAndSetDefault("network_config.enable_ebpf_conntracker", true)
	config.BindEnvAndSetDefault("network_config.enable_cilium_lb_conntracker", true)

	config.BindEnvAndSetDefault("system_probe_config.source_excludes", map[string][]string{})
	config.BindEnvAndSetDefault("system_probe_config.dest_excludes", map[string][]string{})

	config.BindEnvAndSetDefault("system_probe_config.language_detection.enabled", false)

	config.BindEnvAndSetDefault("system_probe_config.process_service_inference.use_improved_algorithm", false)

	// For backward compatibility. Default is false because the canonical key
	// (system_probe_config.process_service_inference.enabled, below) is the
	// authoritative source; deprecateBool only forwards this deprecated alias
	// when it is explicitly configured.
	config.BindEnvAndSetDefault("service_monitoring_config.process_service_inference.enabled", false, "DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_ENABLED")
	config.BindEnvAndSetDefault("system_probe_config.process_service_inference.enabled", GetPlatformDefault(map[string]interface{}{
		"windows": true,
		"other":   false,
	}))

	// For backward compatibility. Default is false because the canonical key
	// (system_probe_config.process_service_inference.use_windows_service_name,
	// below) is the authoritative source; deprecateBool only forwards this
	// deprecated alias when it is explicitly configured.
	config.BindEnvAndSetDefault("service_monitoring_config.process_service_inference.use_windows_service_name", false, "DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_USE_WINDOWS_SERVICE_NAME")
	// default on windows is now enabled; default on linux is still disabled
	config.BindEnvAndSetDefault("system_probe_config.process_service_inference.use_windows_service_name", true)

	// network_config namespace only
	config.BindEnvAndSetDefault("network_config.enable_gateway_lookup", true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP")

	config.BindEnvAndSetDefault("system_probe_config.expected_tags_duration", 30*time.Minute, "DD_SYSTEM_PROBE_EXPECTED_TAGS_DURATION")

	// list of DNS query types to be recorded
	config.BindEnvAndSetDefault("network_config.dns_recorded_query_types", []string{})
	// (temporary) enable submitting DNS stats by query type.
	config.BindEnvAndSetDefault("network_config.enable_dns_by_querytype", false)
	// connection aggregation with port rollups
	config.BindEnvAndSetDefault("network_config.enable_connection_rollup", false)

	config.BindEnvAndSetDefault("network_config.enable_ebpfless", false, "DD_ENABLE_EBPFLESS", "DD_NETWORK_CONFIG_ENABLE_EBPFLESS")

	config.BindEnvAndSetDefault("network_config.enable_co_re", true)
	config.BindEnvAndSetDefault("network_config.enable_fentry", false)
	config.BindEnvAndSetDefault("network_config.enable_sk_tracer", false)

	// TLS cert collection
	config.BindEnvAndSetDefault("network_config.enable_cert_collection", false)
	config.BindEnvAndSetDefault("network_config.cert_collection_map_cleaner_interval", 30*time.Second)

	// windows config
	config.BindEnvAndSetDefault("system_probe_config.windows.enable_monotonic_count", false)

	// oom_kill module
	config.BindEnvAndSetDefault("system_probe_config.enable_oom_kill", false)

	// tcp_queue_length module
	config.BindEnvAndSetDefault("system_probe_config.enable_tcp_queue_length", false)
	// process module
	// nested within system_probe_config to not conflict with process-agent's process_config
	config.BindEnvAndSetDefault("system_probe_config.process_config.enabled", false, "DD_SYSTEM_PROBE_PROCESS_ENABLED")
	// ebpf module
	config.BindEnvAndSetDefault("ebpf_check.enabled", false)
	config.BindEnvAndSetDefault("ebpf_check.kernel_bpf_stats", false)
	// noisy neighbor module
	config.BindEnvAndSetDefault("noisy_neighbor.enabled", false)
	// Per-PMU-event toggles. Default false because each enabled event
	// adds non-trivial overhead.
	config.BindEnvAndSetDefault("noisy_neighbor.pmu_metrics.cycles", false)
	config.BindEnvAndSetDefault("noisy_neighbor.pmu_metrics.instructions", false)
	config.BindEnvAndSetDefault("noisy_neighbor.pmu_metrics.cache_misses", false)
	config.BindEnvAndSetDefault("noisy_neighbor.pmu_metrics.cache_references", false)
	config.BindEnvAndSetDefault("noisy_neighbor.pmu_metrics.itlb_misses", false)
	config.BindEnvAndSetDefault("noisy_neighbor.pmu_metrics.branch_misses", false)
	config.BindEnvAndSetDefault("noisy_neighbor.pmu_metrics.cpu_migrations", false)

	// settings for the entry count of the ebpfcheck
	// control the size of the buffers used for the batch lookups of the ebpf maps
	config.BindEnvAndSetDefault("ebpf_check.entry_count.max_keys_buffer_size_bytes", 512*1024)
	config.BindEnvAndSetDefault("ebpf_check.entry_count.max_values_buffer_size_bytes", 1024*1024)
	// How many times we can restart the entry count of a map before we give up if we get an iteration restart
	// due to the map changing while we look it up
	config.BindEnvAndSetDefault("ebpf_check.entry_count.max_restarts", 3)
	// How many entries we should keep track of in the entry count map to detect restarts in the
	// single-item iteration
	config.BindEnvAndSetDefault("ebpf_check.entry_count.entries_for_iteration_restart_detection", 100)

	// event monitoring
	config.BindEnvAndSetDefault("event_monitoring_config.network_process.enabled", true, "DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED")
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.enable_all_probes", false)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.enable_kernel_filters", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.enable_approvers", false)  // will be set to true by sanitize() if enable_kernel_filters is true
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.enable_discarders", false) // will be set to true by sanitize() if enable_kernel_filters is true
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.basename_approvers_size", 4096)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.flush_discarder_window", 3)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.pid_cache_size", 10000)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.dns_resolution.cache_size", 1024)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.dns_resolution.enabled", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.dns_resolution.cname_max_depth", 2)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.events_stats.tags_cardinality", "high")
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.custom_sensitive_words", []string{})
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.custom_sensitive_regexps", []string{})
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.erpc_dentry_resolution_enabled", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.map_dentry_resolution_enabled", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.dentry_cache_size", 8000)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.lazy_interface_prefixes", []string{})
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.classifier_priority", 10)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.classifier_handle", 0)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.flow_monitor.enabled", false)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.flow_monitor.sk_storage.enabled", false)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.flow_monitor.period", "10s")
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.raw_classifier_handle", 0)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.event_stream.use_ring_buffer", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.event_stream.use_fentry", false)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.event_stream.use_kprobe_fallback", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.event_stream.buffer_size", 0)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.event_stream.kretprobe_max_active", 512)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.envs_with_value", []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "PATH", "HISTSIZE", "HISTFILESIZE", "GLIBC_TUNABLES", "SSH_CLIENT", "DD_SERVICE", "OTEL_SERVICE_NAME", "CLAUDECODE", "RUNNER_TRACKING_ID"})
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.runtime_compilation.enabled", false)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.enabled", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.ingress.enabled", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.raw_packet.enabled", true)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.raw_packet.limiter_rate", 10)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.raw_packet.filter", "no_pid_tcp_syn")
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.private_ip_ranges", DefaultPrivateIPCIDRs)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network.extra_private_ip_ranges", []string{})
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.events_stats.polling_interval", 20)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.syscalls_monitor.enabled", false)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.span_tracking.enabled", false)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.span_tracking.cache_size", 4096)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.capabilities_monitoring.enabled", false)
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.capabilities_monitoring.period", "5s")
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.snapshot_using_listmount", false)
	config.BindEnvAndSetDefault("event_monitoring_config.env_vars_resolution.enabled", true)

	// process event monitoring data limits for network tracer
	// 1024 mirrors defaultMaxProcessesTracked enforced by validateInt in pkg/system-probe/config/adjust_npm.go
	eventMonitorBindEnvAndSetDefault(config, "event_monitoring_config.network_process.max_processes_tracked", 1024)

	config.BindEnvAndSetDefault("event_monitoring_config.network_process.container_store.enabled", true)
	config.BindEnvAndSetDefault("event_monitoring_config.network_process.container_store.max_containers_tracked", 1024)

	config.BindEnvAndSetDefault("compliance_config.database_benchmarks.enabled", false)

	// enable/disable use of root net namespace
	config.BindEnvAndSetDefault("network_config.enable_root_netns", true)

	// Windows crash detection
	config.BindEnvAndSetDefault("windows_crash_detection.enabled", false)

	// Ping
	config.BindEnvAndSetDefault("ping.enabled", false)

	// Traceroute
	config.BindEnvAndSetDefault("traceroute.enabled", false)

	// CCM config
	config.BindEnvAndSetDefault("ccm_network_config.enabled", false)

	// Discovery config
	config.BindEnvAndSetDefault("discovery.enabled", GetPlatformDefault(map[string]interface{}{
		"fargate": false,
		"linux":   true,
		"other":   false,
	}))
	config.BindEnvAndSetDefault("discovery.use_system_probe_lite", GetPlatformDefault(map[string]interface{}{
		"linux": true,
		"other": false,
	}))
	config.BindEnvAndSetDefault("discovery.cpu_usage_update_delay", "60s")
	config.BindEnvAndSetDefault("discovery.service_collection_interval", "60s")
	config.BindEnvAndSetDefault("discovery.service_collection_batch_size", 500)
	config.BindEnvAndSetDefault("discovery.service_collection_max_consecutive_timeouts", 5)
	config.BindEnvAndSetDefault("discovery.service_collection_min_process_age", time.Minute)
	config.BindEnvAndSetDefault("discovery.service_map.enabled", false)

	// Privileged Logs config
	config.BindEnvAndSetDefault("privileged_logs.enabled", false)

	// Logon Duration config (macOS)
	config.BindEnvAndSetDefault("logon_duration.enabled", false)

	// Fleet policies
	config.BindEnvAndSetDefault("fleet_policies_dir", "")

	// GPU monitoring
	config.BindEnvAndSetDefault("gpu_monitoring.enabled", false)
	config.BindEnvAndSetDefault("gpu_monitoring.enable_ebpf_probes", true)
	config.BindEnvAndSetDefault("gpu_monitoring.nvml_lib_path", "")
	config.BindEnvAndSetDefault("gpu_monitoring.process_scan_interval_seconds", 5)
	config.BindEnvAndSetDefault("gpu_monitoring.initial_process_sync", true)
	config.BindEnvAndSetDefault("gpu_monitoring.configure_cgroup_perms", false)
	config.BindEnvAndSetDefault("gpu_monitoring.prm_endpoint_enabled", true)
	config.BindEnvAndSetDefault("gpu_monitoring.enable_fatbin_parsing", false)
	config.BindEnvAndSetDefault("gpu_monitoring.fatbin_request_queue_size", 100)
	config.BindEnvAndSetDefault("gpu_monitoring.ring_buffer_pages_per_device", 32) // 32 pages = 128KB by default per device
	config.BindEnvAndSetDefault("gpu_monitoring.ringbuffer_wakeup_size", 3000)     // 3000 bytes is about ~10-20 events depending on the specific type
	config.BindEnvAndSetDefault("gpu_monitoring.attacher_detailed_logs", false)
	config.BindEnvAndSetDefault("gpu_monitoring.ringbuffer_flush_interval", 1*time.Second)
	config.BindEnvAndSetDefault("gpu_monitoring.device_cache_refresh_interval", 5*time.Second)
	config.BindEnvAndSetDefault("gpu_monitoring.cgroup_reapply_interval", 30*time.Second)
	config.BindEnvAndSetDefault("gpu_monitoring.cgroup_reapply_infinitely", false)

	// Windows Injector telemetry, enabled by default
	config.BindEnvAndSetDefault("injector.enable_telemetry", true)

	// gpu - stream config
	config.BindEnvAndSetDefault("gpu_monitoring.streams.max_kernel_launches", 1000)
	config.BindEnvAndSetDefault("gpu_monitoring.streams.max_mem_alloc_events", 1000)
	config.BindEnvAndSetDefault("gpu_monitoring.streams.max_pending_kernel_spans", 1000)
	config.BindEnvAndSetDefault("gpu_monitoring.streams.max_pending_memory_spans", 1000)
	config.BindEnvAndSetDefault("gpu_monitoring.streams.max_active", 100)
	config.BindEnvAndSetDefault("gpu_monitoring.streams.timeout_seconds", 30) // 30 seconds by default, includes two checks at the default interval of 15 seconds

	config.BindEnvAndSetDefault("network_config.direct_send", false)
}

func initCWSSystemProbeConfig(config pkgconfigmodel.Setup) {
	// CWS - general config
	// the following entries are platform specific
	// - runtime_security_config.policies.dir
	// - runtime_security_config.socket
	config.BindEnvAndSetDefault("runtime_security_config.socket", GetPlatformDefault(map[string]interface{}{
		"windows": "localhost:3335",
		"other":   "${install_path}/run/runtime-security.sock",
	}))
	config.BindEnvAndSetDefault("runtime_security_config.policies.dir", GetPlatformDefault(map[string]interface{}{
		"other":   DefaultRuntimePoliciesDir,
		"windows": "${conf_path}/runtime-security.d",
	}))

	// CWS - general config
	config.BindEnvAndSetDefault("runtime_security_config.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.fim_enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.policies.monitor.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.policies.monitor.per_rule_enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.policies.monitor.report_internal_policies", false)
	config.BindEnvAndSetDefault("runtime_security_config.policies.rule_cache_enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.event_server.burst", 40)
	config.BindEnvAndSetDefault("runtime_security_config.event_server.retention", "6s")
	config.BindEnvAndSetDefault("runtime_security_config.event_server.rate", 10)
	config.BindEnvAndSetDefault("runtime_security_config.event_retry_queue_threshold", 512)
	config.BindEnvAndSetDefault("runtime_security_config.cookie_cache_size", 100)
	config.BindEnvAndSetDefault("runtime_security_config.internal_monitoring.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.log_patterns", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.log_tags", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.self_test.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.self_test.send_report", true)
	config.BindEnvAndSetDefault("runtime_security_config.remote_configuration.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.remote_configuration.dump_policies", false)
	config.BindEnvAndSetDefault("runtime_security_config.direct_send_from_system_probe", false)
	config.BindEnvAndSetDefault("runtime_security_config.event_grpc_server", "")
	config.BindEnvAndSetDefault("runtime_security_config.use_secruntime_track", true)
	config.BindEnvAndSetDefault("runtime_security_config.compliance_module.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.on_demand.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.on_demand.rate_limiter.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.reduced_proc_pid_cache_size", false)
	config.BindEnvAndSetDefault("runtime_security_config.env_as_tags", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.container_exclude", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.container_include", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.exclude_pause_container", true)
	config.BindEnvAndSetDefault("runtime_security_config.cmd_socket", "")

	config.SetDefault("runtime_security_config.windows_filename_cache_max", 16384)
	config.SetDefault("runtime_security_config.windows_registry_cache_max", 4096)
	// windows specific channel size for etw events
	config.SetDefault("runtime_security_config.etw_events_channel_size", 16384)
	config.SetDefault("runtime_security_config.windows_probe_block_on_channel_send", false)
	config.SetDefault("runtime_security_config.windows_write_event_rate_limiter_max_allowed", 4096)
	config.SetDefault("runtime_security_config.windows_write_event_rate_limiter_period", "1s")

	// CWS - activity dump
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.trace_systemd_cgroups", false)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.cleanup_period", "30s")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.tags_resolution_period", "60s")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.load_controller_period", "60s")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.min_timeout", "10m")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.max_dump_size", 1750)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.traced_cgroups_count", 5)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.traced_event_types", []string{"exec", "open", "dns", "imds"})
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_dump_timeout", 900) // deprecated in favor of dump_duration
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.dump_duration", "900s")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.rate_limiter", 500)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_wait_list_timeout", "4500s")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_differentiate_args", false)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.max_dumps_count", 100)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.output_directory", "${run_path}/runtime-security/profiles")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.formats", []string{"profile"})
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.compression", false)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.syscall_monitor.period", "60s")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.max_dump_count_per_workload", 25)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.tag_rules.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.silent_workloads.delay", "10s")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.silent_workloads.ticker", "10s")
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.workload_deny_list", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.activity_dump.auto_suppression.enabled", true)

	// CWS - SBOM
	config.BindEnvAndSetDefault("runtime_security_config.sbom.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.sbom.workloads_cache_size", 10)
	config.BindEnvAndSetDefault("runtime_security_config.sbom.enrichment_interval", "1m")
	config.BindEnvAndSetDefault("runtime_security_config.sbom.refresh_interval", "3s")
	config.BindEnvAndSetDefault("runtime_security_config.sbom.forward_interval", "20s")
	config.BindEnvAndSetDefault("runtime_security_config.sbom.host.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.sbom.generate_policies", false)

	// CWS - Event sampling (per-type)
	config.BindEnvAndSetDefault("runtime_security_config.event_sampling.open.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.event_sampling.open.rate", 500)
	config.BindEnvAndSetDefault("runtime_security_config.event_sampling.connect.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.event_sampling.connect.rate", 500)
	config.BindEnvAndSetDefault("runtime_security_config.event_sampling.bind.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.event_sampling.bind.rate", 500)
	config.BindEnvAndSetDefault("runtime_security_config.event_sampling.dns.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.event_sampling.dns.rate", 500)

	// CWS - Security Profiles
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.v2.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.max_image_tags", 20)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.dir", "${run_path}/runtime-security/profiles")
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.watch_dir", true)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.cache_size", 10)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.max_count", 400)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.dns_match_max_depth", 3)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.node_eviction_timeout", "0s") // Disabled for now - waiting for another PR to be merged
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.profile_cleanup_delay", "60m")

	// CWS - Security Profile V2
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.v2.event_types", []string{"exec", "dns", "bind", "connect", "open"})
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.v2.sample_refresh_period", "30s")
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.v2.excluded_images", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.v2.max_dump_size", 5120)

	// CWS - Auto suppression
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.auto_suppression.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.auto_suppression.event_types", []string{"exec", "dns"})

	// CWS - Anomaly detection
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.event_types", []string{"exec"})
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.default_minimum_stable_period", "900s")
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.minimum_stable_period.exec", "900s")
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.minimum_stable_period.dns", "900s")
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.workload_warmup_period", "180s")
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.unstable_profile_time_threshold", "1h")
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.unstable_profile_size_threshold", 5000000)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.period", "1m")
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.num_keys", 256)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.num_events_allowed", 10)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.tag_rules.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.silent_rule_events.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.enabled", true)

	// CWS - Hash algorithms
	config.BindEnvAndSetDefault("runtime_security_config.hash_resolver.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.hash_resolver.event_types", []string{"exec", "open"})
	config.BindEnvAndSetDefault("runtime_security_config.hash_resolver.max_file_size", (1<<20)*5) // 5 MB
	config.BindEnvAndSetDefault("runtime_security_config.hash_resolver.max_hash_rate", 500)
	config.BindEnvAndSetDefault("runtime_security_config.hash_resolver.hash_algorithms", []string{"sha1", "sha256", "ssdeep"})
	config.BindEnvAndSetDefault("runtime_security_config.hash_resolver.cache_size", 500)
	config.BindEnvAndSetDefault("runtime_security_config.hash_resolver.replace", map[string]string{})

	// CWS - SysCtl
	config.BindEnvAndSetDefault("runtime_security_config.sysctl.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.sysctl.ebpf.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.sysctl.snapshot.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.sysctl.snapshot.period", "1h")
	config.BindEnvAndSetDefault("runtime_security_config.sysctl.snapshot.ignored_base_names", []string{"netdev_rss_key", "stable_secret"})
	config.BindEnvAndSetDefault("runtime_security_config.sysctl.snapshot.kernel_compilation_flags", []string{})

	// CWS - UserSessions
	config.BindEnvAndSetDefault("runtime_security_config.user_sessions.ssh.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.user_sessions.cache_size", 1024)

	// CWS - Capture all syscall errors
	// When enabled, the eBPF IS_UNHANDLED_ERROR filter treats every negative syscall
	// return as handled (constant patched at probe load). Defaults to false.
	config.BindEnvAndSetDefault("runtime_security_config.syscalls.capture_all_errors.enabled", false)

	// CWS -eBPF Less
	config.BindEnvAndSetDefault("runtime_security_config.ebpfless.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.ebpfless.socket", constants.DefaultEBPFLessProbeAddr)

	// CWS - IMDS
	config.BindEnvAndSetDefault("runtime_security_config.imds_ipv4", "169.254.169.254")

	// CWS enforcement capabilities
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.raw_syscall.enabled", false)
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.exclude_binaries", []string{})
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.rule_source_allowed", []string{"file", "remote-config"})
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.container.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.container.max_allowed", 5)
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.container.period", "1m")
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.executable.enabled", true)
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.executable.max_allowed", 5)
	config.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.executable.period", "1m")

	// CWS - File metadata
	config.BindEnvAndSetDefault("runtime_security_config.file_metadata_resolver.enabled", false)

	config.BindEnvAndSetDefault("runtime_security_config.network_monitoring.enabled", false)
}

func initUSMSystemProbeConfig(config pkgconfigmodel.Setup) {
	// ========================================
	// General USM Configuration
	// ========================================
	config.BindEnvAndSetDefault("service_monitoring_config.enabled", false, "DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED")
	// max_concurrent_requests default of 0 is intentional: adjustUSM applies a
	// dynamic default (max_tracked_connections) via applyDefault when this key
	// is not configured by the user.
	config.BindEnvAndSetDefault("service_monitoring_config.max_concurrent_requests", 0)
	config.BindEnvAndSetDefault("service_monitoring_config.enable_quantization", false)
	config.BindEnvAndSetDefault("service_monitoring_config.enable_connection_rollup", false)
	config.BindEnvAndSetDefault("service_monitoring_config.enable_ring_buffers", true)
	config.BindEnvAndSetDefault("service_monitoring_config.enable_event_stream", true)
	// kernel_buffer_pages determines the number of pages allocated *per CPU*
	// for buffering kernel data, whether using a perf buffer or a ring buffer.
	config.BindEnvAndSetDefault("service_monitoring_config.kernel_buffer_pages", 16)
	// data_channel_size defines the size of the Go channel that buffers events.
	// Each event has a fixed size of approximately 4KB (sizeof(batch_data_t)).
	// By setting this value to 100, the channel will buffer up to ~400KB of data in the Go heap memory.
	config.BindEnvAndSetDefault("service_monitoring_config.data_channel_size", 100)
	config.BindEnvAndSetDefault("service_monitoring_config.disable_map_preallocation", true)
	config.BindEnvAndSetDefault("service_monitoring_config.direct_consumer.buffer_wakeup_count_per_cpu", 8)
	config.BindEnvAndSetDefault("service_monitoring_config.direct_consumer.channel_size", 1000)
	config.BindEnvAndSetDefault("service_monitoring_config.direct_consumer.kernel_buffer_size_per_cpu", 65536) // 64KB per CPU base size

	// ========================================
	// HTTP Protocol Configuration
	// ========================================
	// New tree structure with backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.http.enabled", true)
	// Deprecated flat keys for backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.enable_http_monitoring", true)
	config.BindEnvAndSetDefault("network_config.enable_http_monitoring", true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING")

	config.BindEnvAndSetDefault("service_monitoring_config.http.max_stats_buffered", 100000)
	// Deprecated flat keys for backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.max_http_stats_buffered", 100000)
	config.BindEnvAndSetDefault("network_config.max_http_stats_buffered", 100000, "DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED")

	config.BindEnvAndSetDefault("service_monitoring_config.http.max_tracked_connections", 1024)
	// Deprecated flat keys for backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.max_tracked_http_connections", 1024)
	config.BindEnvAndSetDefault("network_config.max_tracked_http_connections", 1024)

	config.BindEnvAndSetDefault("service_monitoring_config.http.notification_threshold", 512)
	// Deprecated flat keys for backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.http_notification_threshold", 512)
	config.BindEnvAndSetDefault("network_config.http_notification_threshold", 512)

	config.BindEnvAndSetDefault("service_monitoring_config.http.max_request_fragment", 512) // matches hard limit currently imposed in NPM driver
	// Deprecated flat keys for backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.http_max_request_fragment", 512)
	config.BindEnvAndSetDefault("network_config.http_max_request_fragment", 512)

	config.BindEnvAndSetDefault("service_monitoring_config.http.map_cleaner_interval_seconds", 300)
	// Deprecated flat keys for backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.http_map_cleaner_interval_in_s", 300)
	config.BindEnvAndSetDefault("system_probe_config.http_map_cleaner_interval_in_s", 300)

	config.BindEnvAndSetDefault("service_monitoring_config.http.idle_connection_ttl_seconds", 30)
	// Deprecated flat keys for backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.http_idle_connection_ttl_in_s", 30)
	config.BindEnvAndSetDefault("system_probe_config.http_idle_connection_ttl_in_s", 30)

	config.BindEnvAndSetDefault("service_monitoring_config.http.use_direct_consumer", true)

	// HTTP replace rules configuration
	config.BindEnvAndSetDefault("service_monitoring_config.http.replace_rules", []map[string]string{})
	// Deprecated flat keys for backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.http_replace_rules", []map[string]string{})
	config.BindEnvAndSetDefault("network_config.http_replace_rules", []map[string]string{}, "DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES")

	config.ParseEnvJSON("service_monitoring_config.http.replace_rules", []map[string]string{})
	config.ParseEnvJSON("service_monitoring_config.http_replace_rules", []map[string]string{})
	config.ParseEnvJSON("network_config.http_replace_rules", []map[string]string{})

	// ========================================
	// HTTP/2 Protocol Configuration
	// ========================================
	// Tree structure
	config.BindEnvAndSetDefault("service_monitoring_config.http2.enabled", false)
	config.BindEnvAndSetDefault("service_monitoring_config.http2.dynamic_table_map_cleaner_interval_seconds", 30)

	// Legacy bindings for backward compatibility (deprecated)
	config.BindEnvAndSetDefault("service_monitoring_config.enable_http2_monitoring", false)
	config.BindEnvAndSetDefault("service_monitoring_config.http2_dynamic_table_map_cleaner_interval_seconds", 30)

	// ========================================
	// Kafka Protocol Configuration
	// ========================================
	config.BindEnvAndSetDefault("service_monitoring_config.kafka.enabled", false)
	// For backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.enable_kafka_monitoring", false)

	config.BindEnvAndSetDefault("service_monitoring_config.kafka.max_stats_buffered", 100000)
	// For backward compatibility
	config.BindEnvAndSetDefault("service_monitoring_config.max_kafka_stats_buffered", 100000)

	// ========================================
	// PostgreSQL Protocol Configuration
	// ========================================
	config.BindEnvAndSetDefault("service_monitoring_config.postgres.enabled", false)
	config.BindEnvAndSetDefault("service_monitoring_config.postgres.max_stats_buffered", 100000)
	config.BindEnvAndSetDefault("service_monitoring_config.postgres.max_telemetry_buffer", 160)

	// ========================================
	// Redis Protocol Configuration
	// ========================================
	config.BindEnvAndSetDefault("service_monitoring_config.redis.enabled", false)
	config.BindEnvAndSetDefault("service_monitoring_config.redis.track_resources", false)
	config.BindEnvAndSetDefault("service_monitoring_config.redis.max_stats_buffered", 100000)

	// ========================================
	// Native TLS Configuration
	// ========================================
	config.BindEnvAndSetDefault("service_monitoring_config.tls.native.enabled", true)
	// For backward compatibility. Default is false because the canonical key
	// (service_monitoring_config.tls.native.enabled, defaulted to true above)
	// is the authoritative source; deprecateBool only forwards the deprecated
	// alias when it is explicitly configured.
	config.BindEnvAndSetDefault("network_config.enable_https_monitoring", false, "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING")

	// ========================================
	// Go TLS Configuration
	// ========================================
	config.BindEnvAndSetDefault("service_monitoring_config.tls.go.enabled", true)
	// For backward compatibility. Default is false because the canonical key
	// (service_monitoring_config.tls.go.enabled, defaulted to true above) is
	// the authoritative source; deprecateBool only forwards the deprecated
	// alias when it is explicitly configured.
	config.BindEnvAndSetDefault("service_monitoring_config.enable_go_tls_support", false)
	config.BindEnvAndSetDefault("service_monitoring_config.tls.go.exclude_self", true)

	// ========================================
	// Istio TLS Configuration
	// ========================================
	config.BindEnvAndSetDefault("service_monitoring_config.tls.istio.enabled", true)
	config.BindEnvAndSetDefault("service_monitoring_config.tls.istio.envoy_path", defaultEnvoyPath)

	// ========================================
	// Node.js TLS Configuration
	// ========================================
	config.BindEnvAndSetDefault("service_monitoring_config.tls.nodejs.enabled", false)
}
