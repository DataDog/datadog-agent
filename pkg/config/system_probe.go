package config

import (
	"strings"
)

const (
	spNS  = "system_probe_config"
	netNS = "network_config"

	defaultConnsMessageBatchSize = 600

	// defaultSystemProbeBPFDir is the default path for eBPF programs
	defaultSystemProbeBPFDir = "/opt/datadog-agent/embedded/share/system-probe/ebpf"

	// defaultRuntimeCompilerOutputDir is the default path for output from the system-probe runtime compiler
	defaultRuntimeCompilerOutputDir = "/var/tmp/datadog-agent/system-probe/build"

	defaultOffsetThreshold = 400
)

func setupSystemProbe(cfg Config) {
	// settings for system-probe in general
	cfg.BindEnvAndSetDefault(join(spNS, "enabled"), false, "DD_SYSTEM_PROBE_ENABLED")
	cfg.BindEnvAndSetDefault(join(spNS, "external"), false, "DD_SYSTEM_PROBE_EXTERNAL")

	cfg.BindEnvAndSetDefault(join(spNS, "sysprobe_socket"), defaultSystemProbeAddress, "DD_SYSPROBE_SOCKET")
	cfg.BindEnvAndSetDefault(join(spNS, "max_conns_per_message"), defaultConnsMessageBatchSize)

	cfg.BindEnvAndSetDefault(join(spNS, "log_file"), defaultSystemProbeLogFilePath)
	cfg.BindEnvAndSetDefault(join(spNS, "log_level"), "info", "DD_LOG_LEVEL", "LOG_LEVEL")
	cfg.BindEnvAndSetDefault(join(spNS, "debug_port"), 0)

	cfg.BindEnvAndSetDefault(join(spNS, "dogstatsd_host"), "127.0.0.1")
	cfg.BindEnvAndSetDefault(join(spNS, "dogstatsd_port"), 8125)

	cfg.BindEnvAndSetDefault(join(spNS, "profiling.enabled"), false, "DD_SYSTEM_PROBE_PROFILING_ENABLED")
	cfg.BindEnvAndSetDefault(join(spNS, "profiling.site"), DefaultSite, "DD_SYSTEM_PROBE_PROFILING_SITE", "DD_SITE")
	cfg.BindEnvAndSetDefault(join(spNS, "profiling.profile_dd_url"), "", "DD_SYSTEM_PROBE_PROFILING_DD_URL", "DD_APM_PROFILING_DD_URL")
	cfg.BindEnvAndSetDefault(join(spNS, "profiling.api_key"), "", "DD_SYSTEM_PROBE_PROFILING_API_KEY", "DD_API_KEY")
	cfg.BindEnvAndSetDefault(join(spNS, "profiling.env"), "", "DD_SYSTEM_PROBE_PROFILING_ENV", "DD_ENV")

	// ebpf general settings
	cfg.BindEnvAndSetDefault(join(spNS, "bpf_debug"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "bpf_dir"), defaultSystemProbeBPFDir)
	cfg.BindEnvAndSetDefault(join(spNS, "excluded_linux_versions"), []string{})
	cfg.BindEnvAndSetDefault(join(spNS, "enable_tracepoints"), false)
	cfg.BindEnvAndSetDefault(join(spNS, "enable_runtime_compiler"), false, "DD_ENABLE_RUNTIME_COMPILER")
	cfg.BindEnvAndSetDefault(join(spNS, "runtime_compiler_output_dir"), defaultRuntimeCompilerOutputDir, "DD_RUNTIME_COMPILER_OUTPUT_DIR")
	cfg.BindEnvAndSetDefault(join(spNS, "kernel_header_dirs"), []string{}, "DD_KERNEL_HEADER_DIRS")

	// network_tracer settings
	cfg.BindEnvAndSetDefault(join(netNS, "enabled"), false, "DD_SYSTEM_PROBE_NETWORK_ENABLED")
	doubleBind(cfg, "disable_tcp", false, "DD_DISABLE_TCP_TRACING")
	doubleBind(cfg, "disable_udp", false, "DD_DISABLE_UDP_TRACING")
	doubleBind(cfg, "disable_ipv6", false, "DD_DISABLE_IPV6_TRACING")
	doubleBind(cfg, "offset_guess_threshold", int64(defaultOffsetThreshold))

	doubleBind(cfg, "max_tracked_connections", 65536)
	doubleBind(cfg, "max_closed_connections_buffered", 50000)
	doubleBind(cfg, "closed_channel_size", 500)
	doubleBind(cfg, "max_connection_state_buffered", 75000)

	doubleBind(cfg, "disable_dns_inspection", false, "DD_DISABLE_DNS_INSPECTION")
	doubleBind(cfg, "collect_dns_stats", true, "DD_COLLECT_DNS_STATS")
	doubleBind(cfg, "collect_local_dns", false, "DD_COLLECT_LOCAL_DNS")
	doubleBind(cfg, "collect_dns_domains", false, "DD_COLLECT_DNS_DOMAINS")
	doubleBind(cfg, "max_dns_stats", 10000)
	doubleBind(cfg, "dns_timeout_in_s", 15)

	doubleBind(cfg, "enable_conntrack", true)
	doubleBind(cfg, "conntrack_max_state_size", 65536*2)
	doubleBind(cfg, "conntrack_rate_limit", 500)
	doubleBind(cfg, "enable_conntrack_all_namespaces", true, "DD_SYSTEM_PROBE_ENABLE_CONNTRACK_ALL_NAMESPACES")
	cfg.BindEnvAndSetDefault(join(netNS, "ignore_conntrack_init_failure"), false, "DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE")

	doubleBind(cfg, "source_excludes", map[string][]string{})
	doubleBind(cfg, "dest_excludes", map[string][]string{})

	// network_config namespace only
	cfg.BindEnvAndSetDefault(join(netNS, "enable_http_monitoring"), false, "DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING")
	cfg.BindEnvAndSetDefault(join(netNS, "enable_gateway_lookup"), false, "DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP")

	// windows config
	doubleBind(cfg, "windows.enable_monotonic_count", false)
	doubleBind(cfg, "windows.driver_buffer_size", 1024)

	// oom_kill module
	cfg.BindEnvAndSetDefault(join(spNS, "enable_oom_kill"), false)
	// tcp_queue_length module
	cfg.BindEnvAndSetDefault(join(spNS, "enable_tcp_queue_length"), false)
	// process module
	// nested within system_probe_config to not conflict with process-agent's process_config
	cfg.BindEnvAndSetDefault(join(spNS, "process_config.enabled"), false, "DD_SYSTEM_PROBE_PROCESS_ENABLED")
}

func join(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// doubleBind configures key in both the `network_config` and `system_probe_config` namespaces.
// This should only be used for existing configuration options, as all new config options
// should be in the `network_config` namespace only.
func doubleBind(cfg Config, key string, val interface{}, env ...string) {
	cfg.BindEnvAndSetDefault(join(netNS, key), val, env...)
	cfg.RegisterAlias(join(spNS, key), join(netNS, key))
}
