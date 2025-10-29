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
	spNS                         = "system_probe_config"
	netNS                        = "network_config"
	smNS                         = "service_monitoring_config"
	evNS                         = "event_monitoring_config"
	compNS                       = "compliance_config"
	ccmNS                        = "ccm_network_config"
	diNS                         = "dynamic_instrumentation"
	wcdNS                        = "windows_crash_detection"
	pngNS                        = "ping"
	tracerouteNS                 = "traceroute"
	discoveryNS                  = "discovery"
	gpuNS                        = "gpu_monitoring"
	privilegedLogsNS             = "privileged_logs"
	swNS                         = "software_inventory"
	defaultConnsMessageBatchSize = 600

	// defaultRuntimeCompilerOutputDir is the default path for output from the system-probe runtime compiler
	defaultRuntimeCompilerOutputDir = "/var/tmp/datadog-agent/system-probe/build"

	// defaultKernelHeadersDownloadDir is the default path for downloading kernel headers for runtime compilation
	defaultKernelHeadersDownloadDir = "/var/tmp/datadog-agent/system-probe/kernel-headers"

	// defaultBTFOutputDir is the default path for extracted BTF
	defaultBTFOutputDir = "/var/tmp/datadog-agent/system-probe/btf"

	// defaultDynamicInstrumentationDebugInfoDir is the default path for debug
	// info for dynamic instrumentation this is the directory where the debug
	// info is decompressed into during processing.
	defaultDynamicInstrumentationDebugInfoDir = "/var/tmp/datadog-agent/system-probe/dynamic-instrumentation/decompressed-debug-info"

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
	cfg.BindEnvAndSetDefault(join(spNS, "disable_thp"), true)

	// SBOM configuration
	cfg.BindEnvAndSetDefault("sbom.host.enabled", false)
	cfg.BindEnvAndSetDefault("sbom.host.analyzers", []string{"os"})
	cfg.BindEnvAndSetDefault("sbom.cache_directory", filepath.Join(defaultRunPath, "sbom-sysprobe"))
	cfg.BindEnvAndSetDefault("sbom.clear_cache_on_exit", false)
	cfg.BindEnvAndSetDefault("sbom.cache.max_disk_size", 1000*1000*100) // used by custom cache: max disk space used by cached objects. Not equal to max disk usage
	cfg.BindEnvAndSetDefault("sbom.cache.clean_interval", "30m")        // used by custom cache.
	cfg.BindEnvAndSetDefault("sbom.scan_queue.base_backoff", "5m")
	cfg.BindEnvAndSetDefault("sbom.scan_queue.max_backoff", "1h")
	// those configs are used by the core agent path, but are not used by the system probe
	cfg.SetKnown("sbom.container_image.overlayfs_direct_scan") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

	// Auto exit configuration
	cfg.BindEnvAndSetDefault("auto_exit.validation_period", 60)
	cfg.BindEnvAndSetDefault("auto_exit.noprocess.enabled", false)
	cfg.BindEnvAndSetDefault("auto_exit.noprocess.excluded_processes", []string{})

	// statsd
	cfg.BindEnv("bind_host") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault("dogstatsd_port", 8125)

	// logging
	cfg.SetKnown(join(spNS, "log_file"))  //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.SetKnown(join(spNS, "log_level")) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
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
	cfg.BindEnvAndSetDefault("secret_backend_output_max_size", 0)
	cfg.BindEnvAndSetDefault("secret_backend_timeout", 0)
	cfg.BindEnvAndSetDefault("secret_backend_command_allow_group_exec_perm", false)
	cfg.BindEnvAndSetDefault("secret_backend_skip_checks", false)

	// settings for system-probe in general
	cfg.BindEnvAndSetDefault(join(spNS, "enabled"), false, "DD_SYSTEM_PROBE_ENABLED")
	cfg.BindEnvAndSetDefault(join(spNS, "external"), false, "DD_SYSTEM_PROBE_EXTERNAL")
	cfg.SetKnown(join(spNS, "adjusted")) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

	cfg.BindEnvAndSetDefault(join(spNS, "sysprobe_socket"), DefaultSystemProbeAddress, "DD_SYSPROBE_SOCKET")
	cfg.BindEnvAndSetDefault(join(spNS, "max_conns_per_message"), defaultConnsMessageBatchSize)

	cfg.BindEnvAndSetDefault(join(spNS, "debug_port"), 0)
	cfg.BindEnvAndSetDefault(join(spNS, "telemetry_enabled"), false, "DD_TELEMETRY_ENABLED")
	cfg.BindEnvAndSetDefault(join(spNS, "telemetry_perf_buffer_emit_per_cpu"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "health_port"), int64(0), "DD_SYSTEM_PROBE_HEALTH_PORT")

	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.enabled"), false, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_ENABLED")
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.site"), DefaultSite, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_SITE", "DD_SITE")
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.profile_dd_url"), "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_DD_URL", "DD_APM_INTERNAL_PROFILING_DD_URL")
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.api_key"), "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_API_KEY", "DD_API_KEY")
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.env"), "", "DD_SYSTEM_PROBE_INTERNAL_PROFILING_ENV", "DD_ENV")
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.period"), 5*time.Minute, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_PERIOD")
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.cpu_duration"), 1*time.Minute, "DD_SYSTEM_PROBE_INTERNAL_PROFILING_CPU_DURATION")
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.mutex_profile_fraction"), 0)
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.block_profile_rate"), 0)
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.enable_goroutine_stacktraces"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.enable_block_profiling"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.enable_mutex_profiling"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.delta_profiles"), true)
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.custom_attributes"), []string{"module", "rule_id"})
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.unix_socket"), "")
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.extra_tags"), []string{})

	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.enabled"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.hierarchy"), "v1")
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.pressure_levels"), map[string]string{})
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.thresholds"), map[string]string{})

	// ebpf general settings
	cfg.BindEnvAndSetDefault(join(spNS, "bpf_debug"), false, "DD_SYSTEM_PROBE_CONFIG_BPF_DEBUG", "BPF_DEBUG")
	cfg.BindEnvAndSetDefault(join(spNS, "bpf_dir"), defaultSystemProbeBPFDir, "DD_SYSTEM_PROBE_BPF_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "excluded_linux_versions"), []string{})
	cfg.BindEnvAndSetDefault(join(spNS, "enable_tracepoints"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "enable_co_re"), true, "DD_ENABLE_CO_RE")
	cfg.BindEnvAndSetDefault(join(spNS, "btf_path"), "", "DD_SYSTEM_PROBE_BTF_PATH")
	cfg.BindEnvAndSetDefault(join(spNS, "btf_output_dir"), defaultBTFOutputDir, "DD_SYSTEM_PROBE_BTF_OUTPUT_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "remote_config_btf_enabled"), false, "DD_SYSTEM_PROBE_REMOTE_CONFIG_BTF_ENABLED")
	cfg.BindEnv(join(spNS, "enable_runtime_compiler"), "DD_ENABLE_RUNTIME_COMPILER") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	// deprecated in favor of allow_prebuilt_fallback below
	cfg.BindEnv(join(spNS, "allow_precompiled_fallback"), "DD_ALLOW_PRECOMPILED_FALLBACK") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv(join(spNS, "allow_prebuilt_fallback"), "DD_ALLOW_PREBUILT_FALLBACK")       //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault(join(spNS, "allow_runtime_compiled_fallback"), true, "DD_ALLOW_RUNTIME_COMPILED_FALLBACK")
	cfg.BindEnvAndSetDefault(join(spNS, "runtime_compiler_output_dir"), defaultRuntimeCompilerOutputDir, "DD_RUNTIME_COMPILER_OUTPUT_DIR")
	cfg.BindEnv(join(spNS, "enable_kernel_header_download"), "DD_ENABLE_KERNEL_HEADER_DOWNLOAD") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault(join(spNS, "kernel_header_dirs"), []string{}, "DD_KERNEL_HEADER_DIRS")
	cfg.BindEnvAndSetDefault(join(spNS, "kernel_header_download_dir"), defaultKernelHeadersDownloadDir, "DD_KERNEL_HEADER_DOWNLOAD_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "apt_config_dir"), suffixHostEtc(defaultAptConfigDirSuffix), "DD_APT_CONFIG_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "yum_repos_dir"), suffixHostEtc(defaultYumReposDirSuffix), "DD_YUM_REPOS_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "zypper_repos_dir"), suffixHostEtc(defaultZypperReposDirSuffix), "DD_ZYPPER_REPOS_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "attach_kprobes_with_kprobe_events_abi"), false, "DD_ATTACH_KPROBES_WITH_KPROBE_EVENTS_ABI")

	// Dynamic Instrumentation settings
	cfg.BindEnvAndSetDefault(join(diNS, "enabled"), false, "DD_DYNAMIC_INSTRUMENTATION_ENABLED")
	cfg.BindEnvAndSetDefault(join(diNS, "offline_mode"), false, "DD_DYNAMIC_INSTRUMENTATION_OFFLINE_MODE")
	cfg.BindEnvAndSetDefault(join(diNS, "probes_file_path"), false, "DD_DYNAMIC_INSTRUMENTATION_PROBES_FILE_PATH")
	cfg.BindEnvAndSetDefault(join(diNS, "snapshot_output_file_path"), false, "DD_DYNAMIC_INSTRUMENTATION_SNAPSHOT_FILE_PATH")
	cfg.BindEnvAndSetDefault(join(diNS, "diagnostics_output_file_path"), false, "DD_DYNAMIC_INSTRUMENTATION_DIAGNOSTICS_FILE_PATH")
	cfg.BindEnvAndSetDefault(join(diNS, "symdb_upload_enabled"), true, "DD_SYMBOL_DATABASE_UPLOAD_ENABLED")
	cfg.BindEnvAndSetDefault(join(diNS, "debug_info_disk_cache", "enabled"), true)
	cfg.BindEnvAndSetDefault(join(diNS, "debug_info_disk_cache", "dir"), defaultDynamicInstrumentationDebugInfoDir)
	cfg.BindEnvAndSetDefault(join(diNS, "debug_info_disk_cache", "max_total_bytes"), int64(2<<30 /* 2GiB */))
	cfg.BindEnvAndSetDefault(join(diNS, "debug_info_disk_cache", "required_disk_space_bytes"), int64(512<<20 /* 512MiB */))
	cfg.BindEnvAndSetDefault(join(diNS, "debug_info_disk_cache", "required_disk_space_percent"), float64(0.0))

	// network_tracer settings
	// we cannot use BindEnvAndSetDefault for network_config.enabled because we need to know if it was manually set.
	cfg.BindEnv(join(netNS, "enabled"), "DD_SYSTEM_PROBE_NETWORK_ENABLED") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv' //nolint:errcheck
	cfg.BindEnvAndSetDefault(join(spNS, "disable_tcp"), false, "DD_DISABLE_TCP_TRACING")
	cfg.BindEnvAndSetDefault(join(spNS, "disable_udp"), false, "DD_DISABLE_UDP_TRACING")
	cfg.BindEnvAndSetDefault(join(spNS, "disable_ipv6"), false, "DD_DISABLE_IPV6_TRACING")

	cfg.SetDefault(join(netNS, "collect_tcp_v4"), true)
	cfg.SetDefault(join(netNS, "collect_tcp_v6"), true)
	cfg.SetDefault(join(netNS, "collect_udp_v4"), true)
	cfg.SetDefault(join(netNS, "collect_udp_v6"), true)

	cfg.BindEnvAndSetDefault(join(spNS, "offset_guess_threshold"), int64(defaultOffsetThreshold))

	cfg.BindEnvAndSetDefault(join(spNS, "max_tracked_connections"), 65536)
	cfg.BindEnv(join(spNS, "max_closed_connections_buffered"))    //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv(join(netNS, "max_failed_connections_buffered"))   //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv(join(spNS, "closed_connection_flush_threshold"))  //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv(join(netNS, "closed_connection_flush_threshold")) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv(join(spNS, "closed_channel_size"))                //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnv(join(netNS, "closed_channel_size"))               //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault(join(netNS, "closed_buffer_wakeup_count"), 4)
	cfg.BindEnvAndSetDefault(join(spNS, "max_connection_state_buffered"), 75000)

	cfg.BindEnvAndSetDefault(join(spNS, "disable_dns_inspection"), false, "DD_DISABLE_DNS_INSPECTION")
	cfg.BindEnvAndSetDefault(join(spNS, "collect_dns_stats"), true, "DD_COLLECT_DNS_STATS")
	cfg.BindEnvAndSetDefault(join(spNS, "collect_local_dns"), false, "DD_COLLECT_LOCAL_DNS")
	cfg.BindEnvAndSetDefault(join(spNS, "collect_dns_domains"), true, "DD_COLLECT_DNS_DOMAINS")
	cfg.BindEnvAndSetDefault(join(spNS, "max_dns_stats"), 20000)
	cfg.BindEnvAndSetDefault(join(spNS, "dns_timeout_in_s"), 15)

	cfg.BindEnvAndSetDefault(join(spNS, "enable_conntrack"), true)
	cfg.BindEnvAndSetDefault(join(spNS, "conntrack_max_state_size"), 65536*2)
	cfg.BindEnvAndSetDefault(join(spNS, "conntrack_rate_limit"), 500)
	cfg.BindEnvAndSetDefault(join(spNS, "enable_conntrack_all_namespaces"), true, "DD_SYSTEM_PROBE_ENABLE_CONNTRACK_ALL_NAMESPACES")
	cfg.BindEnvAndSetDefault(join(netNS, "enable_protocol_classification"), true, "DD_ENABLE_PROTOCOL_CLASSIFICATION")
	cfg.BindEnvAndSetDefault(join(netNS, "enable_ringbuffers"), true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_RINGBUFFERS")
	cfg.BindEnvAndSetDefault(join(netNS, "enable_custom_batching"), false, "DD_SYSTEM_PROBE_NETWORK_ENABLE_CUSTOM_BATCHING")
	cfg.BindEnvAndSetDefault(join(netNS, "enable_tcp_failed_connections"), true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_FAILED_CONNS")
	cfg.BindEnvAndSetDefault(join(netNS, "ignore_conntrack_init_failure"), false, "DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE")
	cfg.BindEnvAndSetDefault(join(netNS, "conntrack_init_timeout"), 10*time.Second)
	cfg.BindEnvAndSetDefault(join(netNS, "allow_netlink_conntracker_fallback"), true)
	cfg.BindEnvAndSetDefault(join(netNS, "enable_ebpf_conntracker"), true)
	cfg.BindEnvAndSetDefault(join(netNS, "enable_cilium_lb_conntracker"), true)

	cfg.BindEnvAndSetDefault(join(spNS, "source_excludes"), map[string][]string{})
	cfg.BindEnvAndSetDefault(join(spNS, "dest_excludes"), map[string][]string{})

	cfg.BindEnvAndSetDefault(join(spNS, "language_detection.enabled"), false)

	cfg.SetKnown(join(spNS, "process_service_inference", "use_improved_algorithm")) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

	// For backward compatibility
	cfg.BindEnv(join(smNS, "process_service_inference", "enabled"), "DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_ENABLED") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault(join(spNS, "process_service_inference", "enabled"), runtime.GOOS == "windows")

	// For backward compatibility
	cfg.BindEnv(join(smNS, "process_service_inference", "use_windows_service_name"), "DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_USE_WINDOWS_SERVICE_NAME") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	// default on windows is now enabled; default on linux is still disabled
	cfg.BindEnvAndSetDefault(join(spNS, "process_service_inference", "use_windows_service_name"), true)

	// network_config namespace only
	cfg.BindEnvAndSetDefault(join(netNS, "enable_gateway_lookup"), true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP")

	cfg.BindEnvAndSetDefault(join(spNS, "expected_tags_duration"), 30*time.Minute, "DD_SYSTEM_PROBE_EXPECTED_TAGS_DURATION")

	// list of DNS query types to be recorded
	cfg.BindEnvAndSetDefault(join(netNS, "dns_recorded_query_types"), []string{})
	// (temporary) enable submitting DNS stats by query type.
	cfg.BindEnvAndSetDefault(join(netNS, "enable_dns_by_querytype"), false)
	// connection aggregation with port rollups
	cfg.BindEnvAndSetDefault(join(netNS, "enable_connection_rollup"), false)

	cfg.BindEnvAndSetDefault(join(netNS, "enable_ebpfless"), false, "DD_ENABLE_EBPFLESS", "DD_NETWORK_CONFIG_ENABLE_EBPFLESS")

	cfg.BindEnvAndSetDefault(join(netNS, "enable_fentry"), false)

	// TLS cert collection
	cfg.BindEnvAndSetDefault(join(netNS, "enable_cert_collection"), false)

	// windows config
	cfg.BindEnvAndSetDefault(join(spNS, "windows.enable_monotonic_count"), false)

	// oom_kill module
	cfg.BindEnvAndSetDefault(join(spNS, "enable_oom_kill"), false)

	// tcp_queue_length module
	cfg.BindEnvAndSetDefault(join(spNS, "enable_tcp_queue_length"), false)
	// process module
	// nested within system_probe_config to not conflict with process-agent's process_config
	cfg.BindEnvAndSetDefault(join(spNS, "process_config.enabled"), false, "DD_SYSTEM_PROBE_PROCESS_ENABLED")
	// ebpf module
	cfg.BindEnvAndSetDefault(join("ebpf_check", "enabled"), false)
	cfg.BindEnvAndSetDefault(join("ebpf_check", "kernel_bpf_stats"), false)

	// settings for the entry count of the ebpfcheck
	// control the size of the buffers used for the batch lookups of the ebpf maps
	cfg.BindEnvAndSetDefault(join("ebpf_check", "entry_count", "max_keys_buffer_size_bytes"), 512*1024)
	cfg.BindEnvAndSetDefault(join("ebpf_check", "entry_count", "max_values_buffer_size_bytes"), 1024*1024)
	// How many times we can restart the entry count of a map before we give up if we get an iteration restart
	// due to the map changing while we look it up
	cfg.BindEnvAndSetDefault(join("ebpf_check", "entry_count", "max_restarts"), 3)
	// How many entries we should keep track of in the entry count map to detect restarts in the
	// single-item iteration
	cfg.BindEnvAndSetDefault(join("ebpf_check", "entry_count", "entries_for_iteration_restart_detection"), 100)

	// event monitoring
	cfg.BindEnvAndSetDefault(join(evNS, "process", "enabled"), false, "DD_SYSTEM_PROBE_EVENT_MONITORING_PROCESS_ENABLED")
	cfg.BindEnvAndSetDefault(join(evNS, "network_process", "enabled"), true, "DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED")
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "enable_all_probes"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "enable_kernel_filters"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "enable_approvers"), false)  // will be set to true by sanitize() if enable_kernel_filters is true
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "enable_discarders"), false) // will be set to true by sanitize() if enable_kernel_filters is true
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "flush_discarder_window"), 3)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "pid_cache_size"), 10000)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "dns_resolution.cache_size"), 1024)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "dns_resolution.enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "events_stats.tags_cardinality"), "high")
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "custom_sensitive_words"), []string{})
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "erpc_dentry_resolution_enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "map_dentry_resolution_enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "dentry_cache_size"), 8000)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.lazy_interface_prefixes"), []string{})
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.classifier_priority"), 10)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.classifier_handle"), 0)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.flow_monitor.enabled"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.flow_monitor.sk_storage.enabled"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.flow_monitor.period"), "10s")
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.raw_classifier_handle"), 0)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "event_stream.use_ring_buffer"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "event_stream.use_fentry"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "event_stream.use_kprobe_fallback"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "event_stream.buffer_size"), 0)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "event_stream.kretprobe_max_active"), 512)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "envs_with_value"), []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "PATH", "HISTSIZE", "HISTFILESIZE", "GLIBC_TUNABLES", "SSH_CLIENT"})
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "runtime_compilation.enabled"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.ingress.enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.raw_packet.enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.raw_packet.limiter_rate"), 10)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.raw_packet.filter"), "no_pid_tcp_syn")
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.private_ip_ranges"), DefaultPrivateIPCIDRs)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.extra_private_ip_ranges"), []string{})
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "events_stats.polling_interval"), 20)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "syscalls_monitor.enabled"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "span_tracking.enabled"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "span_tracking.cache_size"), 4096)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "capabilities_monitoring.enabled"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "capabilities_monitoring.period"), "5s")
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "snapshot_using_listmount"), false)
	cfg.BindEnvAndSetDefault(join(evNS, "socket"), defaultEventMonitorAddress)
	cfg.BindEnvAndSetDefault(join(evNS, "event_server.burst"), 40)
	cfg.BindEnvAndSetDefault(join(evNS, "env_vars_resolution.enabled"), true)

	// process event monitoring data limits for network tracer
	eventMonitorBindEnv(cfg, join(evNS, "network_process", "max_processes_tracked"))

	cfg.BindEnvAndSetDefault(join(compNS, "enabled"), false)

	// enable/disable use of root net namespace
	cfg.BindEnvAndSetDefault(join(netNS, "enable_root_netns"), true)

	// Windows crash detection
	cfg.BindEnvAndSetDefault(join(wcdNS, "enabled"), false)

	// Ping
	cfg.BindEnvAndSetDefault(join(pngNS, "enabled"), false)

	// Traceroute
	cfg.BindEnvAndSetDefault(join(tracerouteNS, "enabled"), false)

	// CCM config
	cfg.BindEnvAndSetDefault(join(ccmNS, "enabled"), false)

	// Discovery config
	cfg.BindEnv(join(discoveryNS, "enabled")) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault(join(discoveryNS, "cpu_usage_update_delay"), "60s")
	cfg.BindEnvAndSetDefault(join(discoveryNS, "ignored_command_names"), []string{"chronyd", "cilium-agent", "containerd", "dhclient", "dockerd", "kubelet", "livenessprobe", "local-volume-pr", "sshd", "systemd"})
	cfg.BindEnvAndSetDefault(join(discoveryNS, "service_collection_interval"), "60s")

	// Privileged Logs config
	cfg.BindEnvAndSetDefault(join(privilegedLogsNS, "enabled"), false)

	// Fleet policies
	cfg.BindEnv("fleet_policies_dir") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'

	// GPU monitoring
	cfg.BindEnvAndSetDefault(join(gpuNS, "enabled"), false)
	cfg.BindEnv(join(gpuNS, "nvml_lib_path")) //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	cfg.BindEnvAndSetDefault(join(gpuNS, "process_scan_interval_seconds"), 5)
	cfg.BindEnvAndSetDefault(join(gpuNS, "initial_process_sync"), true)
	cfg.BindEnvAndSetDefault(join(gpuNS, "configure_cgroup_perms"), false)
	cfg.BindEnvAndSetDefault(join(gpuNS, "enable_fatbin_parsing"), false)
	cfg.BindEnvAndSetDefault(join(gpuNS, "fatbin_request_queue_size"), 100)
	cfg.BindEnvAndSetDefault(join(gpuNS, "ring_buffer_pages_per_device"), 32) // 32 pages = 128KB by default per device
	cfg.BindEnvAndSetDefault(join(gpuNS, "ringbuffer_wakeup_size"), 3000)     // 3000 bytes is about ~10-20 events depending on the specific type
	cfg.BindEnvAndSetDefault(join(gpuNS, "attacher_detailed_logs"), false)
	cfg.BindEnvAndSetDefault(join(gpuNS, "ringbuffer_flush_interval"), 1*time.Second)
	cfg.BindEnvAndSetDefault(join(gpuNS, "device_cache_refresh_interval"), 5*time.Second)

	// gpu - stream config
	cfg.BindEnvAndSetDefault(join(gpuNS, "streams", "max_kernel_launches"), 1000)
	cfg.BindEnvAndSetDefault(join(gpuNS, "streams", "max_mem_alloc_events"), 1000)
	cfg.BindEnvAndSetDefault(join(gpuNS, "streams", "max_pending_kernel_spans"), 1000)
	cfg.BindEnvAndSetDefault(join(gpuNS, "streams", "max_pending_memory_spans"), 1000)
	cfg.BindEnvAndSetDefault(join(gpuNS, "streams", "max_active"), 100)
	cfg.BindEnvAndSetDefault(join(gpuNS, "streams", "timeout_seconds"), 30) // 30 seconds by default, includes two checks at the default interval of 15 seconds

	initCWSSystemProbeConfig(cfg)
	initUSMSystemProbeConfig(cfg)
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
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
