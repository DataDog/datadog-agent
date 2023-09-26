// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

func initCWSSystemProbeConfig(cfg Config) {
	// CWS - general config
	cfg.BindEnvAndSetDefault("runtime_security_config.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.fim_enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.dir", DefaultRuntimePoliciesDir)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.watch_dir", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.monitor.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.policies.monitor.per_rule_enabled", false)
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
	cfg.BindEnvAndSetDefault("runtime_security_config.remote_configuration.enabled", true)
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
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.formats", []string{"profile"})
	cfg.BindEnvAndSetDefault("runtime_security_config.activity_dump.local_storage.compression", false)
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
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.remote_configuration.enabled", false)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.dns_match_max_depth", 3)

	// CWS - Anomaly detection
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.event_types", []string{"exec", "dns"})
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.default_minimum_stable_period", "48h")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.minimum_stable_period.dns", "96h")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.workload_warmup_period", "180s")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.unstable_profile_time_threshold", "120h")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.unstable_profile_size_threshold", 5000000)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.period", "1s")
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.num_keys", 400)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.rate_limiter.num_events_allowed", 100)
	cfg.BindEnvAndSetDefault("runtime_security_config.security_profile.anomaly_detection.tag_rules.enabled", true)

	// CWS - Hash algorithms
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.enabled", true)
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.event_types", []string{"exec", "open"})
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.max_file_size", (1<<20)*10) // 10 MB
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.max_hash_rate", 500)
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.max_hash_burst", 1000)
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.hash_algorithms", []string{"sha1", "sha256"})
	cfg.BindEnvAndSetDefault("runtime_security_config.hash_resolver.cache_size", 500)
}
