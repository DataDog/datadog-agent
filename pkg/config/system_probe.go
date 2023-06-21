// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type transformerFunction func(string) interface{}

const (
	spNS                         = "system_probe_config"
	netNS                        = "network_config"
	smNS                         = "service_monitoring_config"
	dsNS                         = "data_streams_config"
	evNS                         = "event_monitoring_config"
	smjtNS                       = smNS + ".java_tls"
	diNS                         = "dynamic_instrumentation"
	defaultConnsMessageBatchSize = 600

	// defaultSystemProbeBPFDir is the default path for eBPF programs
	defaultSystemProbeBPFDir = "/opt/datadog-agent/embedded/share/system-probe/ebpf"

	// defaultSystemProbeJavaDir is the default path for java agent program
	defaultSystemProbeJavaDir = "/opt/datadog-agent/embedded/share/system-probe/java"

	// defaultServiceMonitoringJavaAgentArgs is default arguments that are passing to the injected java USM agent
	defaultServiceMonitoringJavaAgentArgs = "dd.appsec.enabled=false,dd.trace.enabled=false,dd.usm.enabled=true"

	// defaultRuntimeCompilerOutputDir is the default path for output from the system-probe runtime compiler
	defaultRuntimeCompilerOutputDir = "/var/tmp/datadog-agent/system-probe/build"

	// defaultKernelHeadersDownloadDir is the default path for downloading kernel headers for runtime compilation
	defaultKernelHeadersDownloadDir = "/var/tmp/datadog-agent/system-probe/kernel-headers"

	// defaultAptConfigDirSuffix is the default path under `/etc` to the apt config directory
	defaultAptConfigDirSuffix = "/apt"

	// defaultYumReposDirSuffix is the default path under `/etc` to the yum repository directory
	defaultYumReposDirSuffix = "/yum.repos.d"

	// defaultZypperReposDirSuffix is the default path under `/etc` to the zypper repository directory
	defaultZypperReposDirSuffix = "/zypp/repos.d"

	defaultOffsetThreshold = 400
)

// InitSystemProbeConfig declares all the configuration values normally read from system-probe.yaml.
func InitSystemProbeConfig(cfg Config) {

	cfg.BindEnvAndSetDefault("ignore_host_etc", false)
	cfg.BindEnvAndSetDefault("go_core_dump", false)

	// SBOM configuration
	cfg.BindEnvAndSetDefault("sbom.host.enabled", false)
	cfg.BindEnvAndSetDefault("sbom.host.analyzers", []string{"os"})
	cfg.BindEnvAndSetDefault("sbom.cache_directory", filepath.Join(defaultRunPath, "sbom-sysprobe"))
	cfg.BindEnvAndSetDefault("sbom.clear_cache_on_exit", false)
	cfg.BindEnvAndSetDefault("sbom.cache.enabled", false)
	cfg.BindEnvAndSetDefault("sbom.cache.max_disk_size", 1000*1000*100) // used by custom cache: max disk space used by cached objects. Not equal to max disk usage
	cfg.BindEnvAndSetDefault("sbom.cache.max_cache_entries", 10000)     // used by custom cache keys stored in memory
	cfg.BindEnvAndSetDefault("sbom.cache.clean_interval", "30m")        // used by custom cache.

	// Auto exit configuration
	cfg.BindEnvAndSetDefault("auto_exit.validation_period", 60)
	cfg.BindEnvAndSetDefault("auto_exit.noprocess.enabled", false)
	cfg.BindEnvAndSetDefault("auto_exit.noprocess.excluded_processes", []string{})

	// statsd
	cfg.BindEnv("bind_host")
	cfg.BindEnvAndSetDefault("dogstatsd_port", 8125)

	// logging
	cfg.SetKnown(join(spNS, "log_file"))
	cfg.SetKnown(join(spNS, "log_level"))
	cfg.BindEnvAndSetDefault("log_file", defaultSystemProbeLogFilePath)
	cfg.BindEnvAndSetDefault("log_level", "info", "DD_LOG_LEVEL", "LOG_LEVEL")
	cfg.BindEnvAndSetDefault("syslog_uri", "")
	cfg.BindEnvAndSetDefault("syslog_rfc", false)
	cfg.BindEnvAndSetDefault("log_to_syslog", false)
	cfg.BindEnvAndSetDefault("log_to_console", true)
	cfg.BindEnvAndSetDefault("log_format_json", false)

	// secrets backend
	cfg.BindEnvAndSetDefault("secret_backend_command", "")
	cfg.BindEnvAndSetDefault("secret_backend_arguments", []string{})
	cfg.BindEnvAndSetDefault("secret_backend_output_max_size", secrets.SecretBackendOutputMaxSize)
	cfg.BindEnvAndSetDefault("secret_backend_timeout", 30)
	cfg.BindEnvAndSetDefault("secret_backend_command_allow_group_exec_perm", false)
	cfg.BindEnvAndSetDefault("secret_backend_skip_checks", false)

	// settings for system-probe in general
	cfg.BindEnvAndSetDefault(join(spNS, "enabled"), false, "DD_SYSTEM_PROBE_ENABLED")
	cfg.BindEnvAndSetDefault(join(spNS, "external"), false, "DD_SYSTEM_PROBE_EXTERNAL")

	cfg.BindEnvAndSetDefault(join(spNS, "sysprobe_socket"), defaultSystemProbeAddress, "DD_SYSPROBE_SOCKET")
	cfg.BindEnvAndSetDefault(join(spNS, "max_conns_per_message"), defaultConnsMessageBatchSize)

	cfg.BindEnvAndSetDefault(join(spNS, "debug_port"), 0)
	cfg.BindEnvAndSetDefault(join(spNS, "telemetry_enabled"), true, "DD_TELEMETRY_ENABLED")

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
	cfg.BindEnvAndSetDefault(join(spNS, "internal_profiling.delta_profiles"), false)

	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.enabled"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.hierarchy"), "v1")
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.pressure_levels"), map[string]string{})
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.thresholds"), map[string]string{})

	// ebpf general settings
	cfg.BindEnvAndSetDefault(join(spNS, "bpf_debug"), false, "DD_SYSTEM_PROBE_CONFIG_BPF_DEBUG", "BPF_DEBUG")
	cfg.BindEnvAndSetDefault(join(spNS, "bpf_dir"), defaultSystemProbeBPFDir, "DD_SYSTEM_PROBE_BPF_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "java_dir"), defaultSystemProbeJavaDir, "DD_SYSTEM_PROBE_JAVA_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "excluded_linux_versions"), []string{})
	cfg.BindEnvAndSetDefault(join(spNS, "enable_tracepoints"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "enable_co_re"), true, "DD_ENABLE_CO_RE")
	cfg.BindEnvAndSetDefault(join(spNS, "btf_path"), "", "DD_SYSTEM_PROBE_BTF_PATH")
	cfg.BindEnv(join(spNS, "enable_runtime_compiler"), "DD_ENABLE_RUNTIME_COMPILER")
	cfg.BindEnvAndSetDefault(join(spNS, "allow_precompiled_fallback"), true, "DD_ALLOW_PRECOMPILED_FALLBACK")
	cfg.BindEnvAndSetDefault(join(spNS, "allow_runtime_compiled_fallback"), true, "DD_ALLOW_RUNTIME_COMPILED_FALLBACK")
	cfg.BindEnvAndSetDefault(join(spNS, "runtime_compiler_output_dir"), defaultRuntimeCompilerOutputDir, "DD_RUNTIME_COMPILER_OUTPUT_DIR")
	cfg.BindEnv(join(spNS, "enable_kernel_header_download"), "DD_ENABLE_KERNEL_HEADER_DOWNLOAD")
	cfg.BindEnvAndSetDefault(join(spNS, "kernel_header_dirs"), []string{}, "DD_KERNEL_HEADER_DIRS")
	cfg.BindEnvAndSetDefault(join(spNS, "kernel_header_download_dir"), defaultKernelHeadersDownloadDir, "DD_KERNEL_HEADER_DOWNLOAD_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "apt_config_dir"), suffixHostEtc(defaultAptConfigDirSuffix), "DD_APT_CONFIG_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "yum_repos_dir"), suffixHostEtc(defaultYumReposDirSuffix), "DD_YUM_REPOS_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "zypper_repos_dir"), suffixHostEtc(defaultZypperReposDirSuffix), "DD_ZYPPER_REPOS_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "attach_kprobes_with_kprobe_events_abi"), false, "DD_ATTACH_KPROBES_WITH_KPROBE_EVENTS_ABI")

	// User Tracer
	cfg.BindEnvAndSetDefault(join(diNS, "enabled"), false, "DD_DYNAMIC_INSTRUMENTATION_ENABLED")

	// network_tracer settings
	// we cannot use BindEnvAndSetDefault for network_config.enabled because we need to know if it was manually set.
	cfg.BindEnv(join(netNS, "enabled"), "DD_SYSTEM_PROBE_NETWORK_ENABLED") //nolint:errcheck
	cfg.BindEnvAndSetDefault(join(spNS, "disable_tcp"), false, "DD_DISABLE_TCP_TRACING")
	cfg.BindEnvAndSetDefault(join(spNS, "disable_udp"), false, "DD_DISABLE_UDP_TRACING")
	cfg.BindEnvAndSetDefault(join(spNS, "disable_ipv6"), false, "DD_DISABLE_IPV6_TRACING")

	cfg.SetDefault(join(netNS, "collect_tcp_v4"), true)
	cfg.SetDefault(join(netNS, "collect_tcp_v6"), true)
	cfg.SetDefault(join(netNS, "collect_udp_v4"), true)
	cfg.SetDefault(join(netNS, "collect_udp_v6"), true)

	cfg.BindEnvAndSetDefault(join(spNS, "offset_guess_threshold"), int64(defaultOffsetThreshold))

	cfg.BindEnvAndSetDefault(join(spNS, "max_tracked_connections"), 65536)
	cfg.BindEnv(join(spNS, "max_closed_connections_buffered"))
	cfg.BindEnvAndSetDefault(join(spNS, "closed_connection_flush_threshold"), 0)
	cfg.BindEnvAndSetDefault(join(spNS, "closed_channel_size"), 500)
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
	cfg.BindEnvAndSetDefault(join(netNS, "ignore_conntrack_init_failure"), false, "DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE")
	cfg.BindEnvAndSetDefault(join(netNS, "conntrack_init_timeout"), 10*time.Second)
	cfg.BindEnvAndSetDefault(join(netNS, "allow_netlink_conntracker_fallback"), true)

	cfg.BindEnvAndSetDefault(join(spNS, "source_excludes"), map[string][]string{})
	cfg.BindEnvAndSetDefault(join(spNS, "dest_excludes"), map[string][]string{})

	// network_config namespace only

	// For backward compatibility
	cfg.BindEnv(join(netNS, "enable_http_monitoring"), "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING")
	cfg.BindEnv(join(smNS, "enable_http_monitoring"))

	cfg.BindEnv(join(netNS, "enable_https_monitoring"), "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING")

	cfg.BindEnvAndSetDefault(join(smNS, "enable_go_tls_support"), false)

	cfg.BindEnvAndSetDefault(join(smNS, "enable_http2_monitoring"), false)
	cfg.BindEnvAndSetDefault(join(smjtNS, "enabled"), false)
	cfg.BindEnvAndSetDefault(join(smjtNS, "debug"), false)
	cfg.BindEnvAndSetDefault(join(smjtNS, "args"), defaultServiceMonitoringJavaAgentArgs)
	cfg.BindEnvAndSetDefault(join(smjtNS, "allow_regex"), "")
	cfg.BindEnvAndSetDefault(join(smjtNS, "block_regex"), "")
	cfg.BindEnvAndSetDefault(join(smNS, "enable_http_stats_by_status_code"), false)

	cfg.BindEnvAndSetDefault(join(netNS, "enable_gateway_lookup"), true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP")
	// Default value (100000) is set in `adjustUSM`, to avoid having "deprecation warning", due to the default value.
	cfg.BindEnv(join(netNS, "max_http_stats_buffered"), "DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED")
	cfg.BindEnv(join(smNS, "max_http_stats_buffered"))
	cfg.BindEnvAndSetDefault(join(smNS, "max_kafka_stats_buffered"), 100000)

	oldHTTPRules := join(netNS, "http_replace_rules")
	newHTTPRules := join(smNS, "http_replace_rules")
	cfg.BindEnv(newHTTPRules)
	cfg.BindEnv(oldHTTPRules, "DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES")
	httpRulesTransformer := func(key string) transformerFunction {
		return func(in string) interface{} {
			var out []map[string]string
			if err := json.Unmarshal([]byte(in), &out); err != nil {
				log.Warnf(`%q can not be parsed: %v`, key, err)
			}
			return out
		}
	}
	cfg.SetEnvKeyTransformer(oldHTTPRules, httpRulesTransformer(oldHTTPRules))
	cfg.SetEnvKeyTransformer(newHTTPRules, httpRulesTransformer(newHTTPRules))

	// Default value (1024) is set in `adjustUSM`, to avoid having "deprecation warning", due to the default value.
	cfg.BindEnv(join(netNS, "max_tracked_http_connections"))
	cfg.BindEnv(join(smNS, "max_tracked_http_connections"))
	// Default value (512) is set in `adjustUSM`, to avoid having "deprecation warning", due to the default value.
	cfg.BindEnv(join(netNS, "http_notification_threshold"))
	cfg.BindEnv(join(smNS, "http_notification_threshold"))
	// Default value (160) is set in `adjustUSM`, to avoid having "deprecation warning", due to the default value.
	cfg.BindEnv(join(netNS, "http_max_request_fragment"))
	cfg.BindEnv(join(smNS, "http_max_request_fragment"))

	// list of DNS query types to be recorded
	cfg.BindEnvAndSetDefault(join(netNS, "dns_recorded_query_types"), []string{})
	// (temporary) enable submitting DNS stats by query type.
	cfg.BindEnvAndSetDefault(join(netNS, "enable_dns_by_querytype"), false)

	// windows config
	cfg.BindEnvAndSetDefault(join(spNS, "windows.enable_monotonic_count"), false)

	// oom_kill module
	cfg.BindEnvAndSetDefault(join(spNS, "enable_oom_kill"), false)
	// tcp_queue_length module
	cfg.BindEnvAndSetDefault(join(spNS, "enable_tcp_queue_length"), false)
	// process module
	// nested within system_probe_config to not conflict with process-agent's process_config
	cfg.BindEnvAndSetDefault(join(spNS, "process_config.enabled"), false, "DD_SYSTEM_PROBE_PROCESS_ENABLED")

	// service monitoring
	cfg.BindEnvAndSetDefault(join(smNS, "enabled"), false, "DD_SYSTEM_PROBE_SERVICE_MONITORING_ENABLED")
	cfg.BindEnvAndSetDefault(join(smNS, "process_service_inference", "enabled"), false, "DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_ENABLED")
	cfg.BindEnvAndSetDefault(join(smNS, "process_service_inference", "use_windows_service_name"), true, "DD_SYSTEM_PROBE_PROCESS_SERVICE_INFERENCE_USE_WINDOWS_SERVICE_NAME")

	// Default value (300) is set in `adjustUSM`, to avoid having "deprecation warning", due to the default value.
	cfg.BindEnv(join(spNS, "http_map_cleaner_interval_in_s"))
	cfg.BindEnv(join(smNS, "http_map_cleaner_interval_in_s"))

	// Default value (30) is set in `adjustUSM`, to avoid having "deprecation warning", due to the default value.
	cfg.BindEnv(join(spNS, "http_idle_connection_ttl_in_s"))
	cfg.BindEnv(join(smNS, "http_idle_connection_ttl_in_s"))

	// data streams
	cfg.BindEnvAndSetDefault(join(dsNS, "enabled"), false, "DD_SYSTEM_PROBE_DATA_STREAMS_ENABLED")

	// event monitoring
	cfg.BindEnvAndSetDefault(join(evNS, "process", "enabled"), false, "DD_SYSTEM_PROBE_EVENT_MONITORING_PROCESS_ENABLED")
	cfg.BindEnvAndSetDefault(join(evNS, "network_process", "enabled"), false, "DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED")
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "enable_kernel_filters"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "flush_discarder_window"), 3)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "pid_cache_size"), 10000)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "events_stats.tags_cardinality"), "high")
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "custom_sensitive_words"), []string{})
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "erpc_dentry_resolution_enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "map_dentry_resolution_enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "dentry_cache_size"), 1024)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "remote_tagger"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "runtime_monitor.enabled"), false)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.lazy_interface_prefixes"), []string{})
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.classifier_priority"), 10)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.classifier_handle"), 0)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "event_stream.use_ring_buffer"), true)
	eventMonitorBindEnv(cfg, join(evNS, "event_stream.buffer_size"))
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "envs_with_value"), []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "PATH", "HISTSIZE", "HISTFILESIZE"})
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "runtime_compilation.enabled"), false)
	eventMonitorBindEnv(cfg, join(evNS, "runtime_compilation.compiled_constants_enabled"))
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "network.enabled"), true)
	eventMonitorBindEnvAndSetDefault(cfg, join(evNS, "events_stats.polling_interval"), 20)
	cfg.BindEnvAndSetDefault(join(evNS, "socket"), "/opt/datadog-agent/run/event-monitor.sock")
	cfg.BindEnvAndSetDefault(join(evNS, "event_server.burst"), 40)

	// process event monitoring data limits for network tracer
	eventMonitorBindEnv(cfg, join(evNS, "network_process", "max_processes_tracked"))

	// enable/disable use of root net namespace
	cfg.BindEnvAndSetDefault(join(netNS, "enable_root_netns"), true)

	// CWS - general config
	cfg.BindEnvAndSetDefault("runtime_security_config.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.fim_enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.dir", DefaultRuntimePoliciesDir)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.watch_dir", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.monitor.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.socket", "/opt/datadog-agent/run/runtime-security.sock")
	cfg.BindEnvAndSetDefault("runtime_security_config.event_server.burst", 40)
	cfg.BindEnvAndSetDefault("runtime_security_config.event_server.retention", "6s")
	cfg.BindEnvAndSetDefault("runtime_security_config.event_server.rate", 10)
	cfg.BindEnvAndSetDefault("runtime_security_config.cookie_cache_size", 100)
	cfg.BindEnvAndSetDefault("runtime_security_config.agent_monitoring_events", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.log_patterns", []string{})
	cfg.BindEnvAndSetDefault("runtime_security_config.log_tags", []string{})
	cfg.BindEnvAndSetDefault("runtime_security_config.self_test.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.self_test.send_report", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.remote_configuration.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.direct_send_from_system_probe", false)

	// CWS - activity dump
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.cleanup_period", "30s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.tags_resolution_period", "60s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.load_controller_period", "60s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.min_timeout", "10m")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.max_dump_size", 1750)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.traced_cgroups_count", 5)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.traced_event_types", []string{"exec", "open", "dns"})
	cfg.BindEnv("runtime_security_config.activity_dump.cgroup_dump_timeout") // deprecated in favor of dump_duration
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.dump_duration", "1800s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.rate_limiter", 500)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_wait_list_timeout", "4500s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_differentiate_args", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.max_dumps_count", 100)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.output_directory", DefaultSecurityProfilesDir)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.formats", []string{})
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.compression", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.syscall_monitor.period", "60s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.max_dump_count_per_workload", 25)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.tag_rules.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.silent_workloads.delay", "10s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.silent_workloads.ticker", "10s")

	// CWS - SBOM
	cfg.BindEnvAndSetDefault("runtime_security_config.sbom.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.sbom.workloads_cache_size", 10)

	// CWS - Security Profiles
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.dir", DefaultSecurityProfilesDir)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.watch_dir", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.cache_size", 10)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.max_count", 400)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.remote_configuration.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.dns_match_max_depth", 0)

	// CWS - Anomaly detection
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.event_types", []string{"exec", "dns"})
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.minimum_stable_period", "24h")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.workload_warmup_period", "60s")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.unstable_profile_time_threshold", "72h")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.unstable_profile_size_threshold", 5000000)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter", "1ms")
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
func eventMonitorBindEnvAndSetDefault(config Config, key string, val interface{}) {
	// Uppercase, replace "." with "_" and add "DD_" prefix to key so that we follow the same environment
	// variable convention as the core agent.
	emConfigKey := "DD_" + strings.Replace(strings.ToUpper(key), ".", "_", -1)
	runtimeSecKey := strings.Replace(emConfigKey, "EVENT_MONITORING_CONFIG", "RUNTIME_SECURITY_CONFIG", 1)

	envs := []string{emConfigKey, runtimeSecKey}
	config.BindEnvAndSetDefault(key, val, envs...)
}

// eventMonitorBindEnv is the same as eventMonitorBindEnvAndSetDefault, but without setting a default.
func eventMonitorBindEnv(config Config, key string) {
	emConfigKey := "DD_" + strings.Replace(strings.ToUpper(key), ".", "_", -1)
	runtimeSecKey := strings.Replace(emConfigKey, "EVENT_MONITORING_CONFIG", "RUNTIME_SECURITY_CONFIG", 1)

	config.BindEnv(key, emConfigKey, runtimeSecKey)
}
