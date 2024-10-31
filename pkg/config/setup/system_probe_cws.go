// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
)

func initCWSSystemProbeConfig(cfg pkgconfigmodel.Config) {
	// CWS - general config
	// the following entries are platform specific
	// - runtime_security_config.policies.dir
	// - runtime_security_config.socket
	platformCWSConfig(cfg)

	// CWS - general config
	cfg.BindEnvAndSetDefault("runtime_security_config.enabled", false)
	cfg.BindEnv("runtime_security_config.fim_enabled")
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.watch_dir", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.monitor.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.monitor.per_rule_enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.monitor.report_internal_policies", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.event_server.burst", 40)
	cfg.BindEnvAndSetDefault("runtime_security_config.event_server.retention", "6s")
	cfg.BindEnvAndSetDefault("runtime_security_config.event_server.rate", 10)
	cfg.BindEnvAndSetDefault("runtime_security_config.cookie_cache_size", 100)
	cfg.BindEnvAndSetDefault("runtime_security_config.internal_monitoring.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.log_patterns", []string{})
	cfg.BindEnvAndSetDefault("runtime_security_config.log_tags", []string{})
	cfg.BindEnvAndSetDefault("runtime_security_config.self_test.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.self_test.send_report", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.remote_configuration.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.remote_configuration.dump_policies", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.direct_send_from_system_probe", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.use_secruntime_track", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.compliance_module.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.on_demand.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.on_demand.rate_limiter.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.reduced_proc_pid_cache_size", false)

	cfg.SetDefault("runtime_security_config.windows_filename_cache_max", 16384)
	cfg.SetDefault("runtime_security_config.windows_registry_cache_max", 4096)
	// windows specific channel size for etw events
	cfg.SetDefault("runtime_security_config.etw_events_channel_size", 128)
	cfg.SetDefault("runtime_security_config.etw_events_max_buffers", 0)
	cfg.SetDefault("runtime_security_config.windows_probe_block_on_channel_send", false)

	// CWS - activity dump
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.cleanup_period", "30s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.tags_resolution_period", "60s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.load_controller_period", "60s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.min_timeout", "10m")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.max_dump_size", 1750)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.traced_cgroups_count", 5)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.traced_event_types", []string{"exec", "open", "dns", "imds"})
	cfg.BindEnv("runtime_security_config.activity_dump.cgroup_dump_timeout") // deprecated in favor of dump_duration
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.dump_duration", "900s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.rate_limiter", 500)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_wait_list_timeout", "4500s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.cgroup_differentiate_args", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.max_dumps_count", 100)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.output_directory", GetDefaultSecurityProfilesDir())
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.formats", []string{"profile"})
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.compression", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.syscall_monitor.period", "60s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.max_dump_count_per_workload", 25)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.tag_rules.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.silent_workloads.delay", "10s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.silent_workloads.ticker", "10s")
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.workload_deny_list", []string{})
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.auto_suppression.enabled", true)

	// CWS - SBOM
	cfg.BindEnvAndSetDefault("runtime_security_config.sbom.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.sbom.workloads_cache_size", 10)
	cfg.BindEnvAndSetDefault("runtime_security_config.sbom.host.enabled", false)

	// CWS - Security Profiles
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.max_image_tags", 20)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.dir", GetDefaultSecurityProfilesDir())
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.watch_dir", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.cache_size", 10)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.max_count", 400)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.dns_match_max_depth", 3)

	// CWS - Auto suppression
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.auto_suppression.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.auto_suppression.event_types", []string{"exec", "dns"})

	// CWS - Anomaly detection
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.event_types", []string{"exec"})
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.default_minimum_stable_period", "900s")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.minimum_stable_period.exec", "900s")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.minimum_stable_period.dns", "900s")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.workload_warmup_period", "180s")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.unstable_profile_time_threshold", "1h")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.unstable_profile_size_threshold", 5000000)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.period", "1m")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.num_keys", 1000)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.num_events_allowed", 300)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.tag_rules.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.silent_rule_events.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.enabled", true)

	// CWS - Hash algorithms
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.event_types", []string{"exec", "open"})
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.max_file_size", (1<<20)*10) // 10 MB
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.max_hash_rate", 500)
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.max_hash_burst", 1000)
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.hash_algorithms", []string{"sha1", "sha256", "ssdeep"})
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.cache_size", 500)
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.replace", map[string]string{})

	// CWS - UserSessions
	cfg.BindEnvAndSetDefault("runtime_security_config.user_sessions.cache_size", 1024)

	// CWS -eBPF Less
	cfg.BindEnvAndSetDefault("runtime_security_config.ebpfless.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.ebpfless.socket", constants.DefaultEBPFLessProbeAddr)

	// CWS - IMDS
	cfg.BindEnvAndSetDefault("runtime_security_config.imds_ipv4", "169.254.169.254")

	// CWS enforcement capabilities
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.raw_syscall.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.exclude_binaries", []string{})
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.rule_source_allowed", []string{"file", "remote-config"})
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.container.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.container.max_allowed", 5)
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.container.period", "1m")
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.executable.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.executable.max_allowed", 5)
	cfg.BindEnvAndSetDefault("runtime_security_config.enforcement.disarmer.executable.period", "1m")

	cfg.BindEnvAndSetDefault("runtime_security_config.network_monitoring.enabled", false)
}
