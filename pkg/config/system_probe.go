// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"os"
	"path"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	spNS  = "system_probe_config"
	netNS = "network_config"
	smNS  = "service_monitoring_config"

	defaultConnsMessageBatchSize = 600

	// defaultSystemProbeBPFDir is the default path for eBPF programs
	defaultSystemProbeBPFDir = "/opt/datadog-agent/embedded/share/system-probe/ebpf"

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

func isSystemProbeConfigInit(cfg Config) bool {
	keys := cfg.GetKnownKeys()
	_, ok := keys[join(spNS, "enabled")]
	return ok
}

// InitSystemProbeConfig declares all the configuration values normally read from system-probe.yaml.
// This function should not be called before ResolveSecrets,
// unless you call `cmd/system-probe/config.New` or `cmd/system-probe/config.Merge` in-between.
// This is to prevent the in-memory values from being fixed before the file-based values have had a chance to be read.
func InitSystemProbeConfig(cfg Config) {
	if isSystemProbeConfigInit(cfg) {
		return
	}

	// settings for system-probe in general
	cfg.BindEnvAndSetDefault(join(spNS, "enabled"), false, "DD_SYSTEM_PROBE_ENABLED")
	cfg.BindEnvAndSetDefault(join(spNS, "external"), false, "DD_SYSTEM_PROBE_EXTERNAL")

	cfg.BindEnvAndSetDefault(join(spNS, "sysprobe_socket"), defaultSystemProbeAddress, "DD_SYSPROBE_SOCKET")
	cfg.BindEnvAndSetDefault(join(spNS, "max_conns_per_message"), defaultConnsMessageBatchSize)

	cfg.BindEnvAndSetDefault(join(spNS, "log_file"), defaultSystemProbeLogFilePath)
	cfg.BindEnvAndSetDefault(join(spNS, "log_level"), "info", "DD_LOG_LEVEL", "LOG_LEVEL")
	cfg.BindEnvAndSetDefault(join(spNS, "debug_port"), 0)
	cfg.BindEnvAndSetDefault(join(spNS, "telemetry_enabled"), true, "DD_TELEMETRY_ENABLED")

	cfg.BindEnvAndSetDefault(join(spNS, "dogstatsd_host"), "127.0.0.1")
	cfg.BindEnvAndSetDefault(join(spNS, "dogstatsd_port"), 8125)

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

	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.enabled"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.hierarchy"), "v1")
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.pressure_levels"), map[string]string{})
	cfg.BindEnvAndSetDefault(join(spNS, "memory_controller.thresholds"), map[string]string{})

	// ebpf general settings
	cfg.BindEnvAndSetDefault(join(spNS, "bpf_debug"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "bpf_dir"), defaultSystemProbeBPFDir, "DD_SYSTEM_PROBE_BPF_DIR")
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

	// network_tracer settings
	// we cannot use BindEnvAndSetDefault for network_config.enabled because we need to know if it was manually set.
	cfg.BindEnv(join(netNS, "enabled"), "DD_SYSTEM_PROBE_NETWORK_ENABLED") //nolint:errcheck
	cfg.BindEnvAndSetDefault(join(spNS, "disable_tcp"), false, "DD_DISABLE_TCP_TRACING")
	cfg.BindEnvAndSetDefault(join(spNS, "disable_udp"), false, "DD_DISABLE_UDP_TRACING")
	cfg.BindEnvAndSetDefault(join(spNS, "disable_ipv6"), false, "DD_DISABLE_IPV6_TRACING")
	cfg.BindEnvAndSetDefault(join(spNS, "offset_guess_threshold"), int64(defaultOffsetThreshold))

	cfg.BindEnvAndSetDefault(join(spNS, "max_tracked_connections"), 65536)
	cfg.BindEnv(join(spNS, "max_closed_connections_buffered"))
	cfg.BindEnvAndSetDefault(join(spNS, "closed_channel_size"), 500)
	cfg.BindEnvAndSetDefault(join(spNS, "max_connection_state_buffered"), 75000)

	cfg.BindEnvAndSetDefault(join(spNS, "disable_dns_inspection"), false, "DD_DISABLE_DNS_INSPECTION")
	cfg.BindEnvAndSetDefault(join(spNS, "collect_dns_stats"), true, "DD_COLLECT_DNS_STATS")
	cfg.BindEnvAndSetDefault(join(spNS, "collect_local_dns"), false, "DD_COLLECT_LOCAL_DNS")
	cfg.BindEnvAndSetDefault(join(spNS, "collect_dns_domains"), true, "DD_COLLECT_DNS_DOMAINS")
	cfg.BindEnvAndSetDefault(join(spNS, "max_dns_stats"), 20000)
	cfg.BindEnvAndSetDefault(join(spNS, "dns_timeout_in_s"), 15)
	cfg.BindEnvAndSetDefault(join(spNS, "http_map_cleaner_interval_in_s"), 300)
	cfg.BindEnvAndSetDefault(join(spNS, "http_idle_connection_ttl_in_s"), 30)

	cfg.BindEnvAndSetDefault(join(spNS, "enable_conntrack"), true)
	cfg.BindEnvAndSetDefault(join(spNS, "conntrack_max_state_size"), 65536*2)
	cfg.BindEnvAndSetDefault(join(spNS, "conntrack_rate_limit"), 500)
	cfg.BindEnvAndSetDefault(join(spNS, "enable_conntrack_all_namespaces"), true, "DD_SYSTEM_PROBE_ENABLE_CONNTRACK_ALL_NAMESPACES")
	cfg.BindEnvAndSetDefault(join(netNS, "enable_protocol_classification"), true, "DD_ENABLE_PROTOCOL_CLASSIFICATION")
	cfg.BindEnvAndSetDefault(join(netNS, "ignore_conntrack_init_failure"), false, "DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE")
	cfg.BindEnvAndSetDefault(join(netNS, "conntrack_init_timeout"), 10*time.Second)

	cfg.BindEnvAndSetDefault(join(spNS, "source_excludes"), map[string][]string{})
	cfg.BindEnvAndSetDefault(join(spNS, "dest_excludes"), map[string][]string{})

	// network_config namespace only
	cfg.BindEnv(join(netNS, "enable_http_monitoring"), "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING")
	cfg.BindEnv(join(netNS, "enable_https_monitoring"), "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTPS_MONITORING")

	cfg.BindEnvAndSetDefault(join(spNS, "enable_go_tls_support"), false)

	cfg.BindEnvAndSetDefault(join(netNS, "enable_gateway_lookup"), true, "DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP")
	cfg.BindEnvAndSetDefault(join(netNS, "max_http_stats_buffered"), 100000, "DD_SYSTEM_PROBE_NETWORK_MAX_HTTP_STATS_BUFFERED")
	httpRules := join(netNS, "http_replace_rules")
	cfg.BindEnv(httpRules, "DD_SYSTEM_PROBE_NETWORK_HTTP_REPLACE_RULES")
	cfg.SetEnvKeyTransformer(httpRules, func(in string) interface{} {
		var out []map[string]string
		if err := json.Unmarshal([]byte(in), &out); err != nil {
			log.Warnf(`%q can not be parsed: %v`, httpRules, err)
		}
		return out
	})
	cfg.BindEnvAndSetDefault(join(netNS, "max_tracked_http_connections"), 1024)
	cfg.BindEnvAndSetDefault(join(netNS, "http_notification_threshold"), 512)
	cfg.BindEnvAndSetDefault(join(netNS, "http_max_request_fragment"), 160)

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

	// enable/disable use of root net namespace
	cfg.BindEnvAndSetDefault(join(netNS, "enable_root_netns"), true)
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
