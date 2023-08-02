// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

// defaultFlareFiles contains all the files that are included in the flare archive by default (no need for a specific configuration option)
var defaultFlareFiles = []string{
	"config-check.log",
	"connectivity.log",
	"diagnose.log",
	"docker_ps.log",
	"envvars.log",
	"go-routine-dump.log",
	"health.yaml",
	"install_info",
	"metadata_inventories.json",
	"metadata_v5.json",
	"permissions.log",
	"process_agent_runtime_config_dump.yaml",
	"process-agent_tagger-list.json",
	"runtime_config_dump.yaml",
	"secrets.log",
	"status.log",
	"system_probe_runtime_config_dump.yaml",
	"tagger-list.json",
	"version-history.json",
	"workload-list.log",
}

var defaultLogFiles = []string{
	"logs/agent.log",
	"logs/process-agent.log",
	"logs/trace-agent.log",
}

var allLogFiles = []string{
	"logs/agent.log",
	"logs/process-agent.log",
	"logs/security-agent.log",
	"logs/system-probe.log",
	"logs/trace-agent.log",
}

var defaultConfigFiles = []string{"etc/datadog.yaml"}

var allConfigFiles = []string{
	"etc/datadog.yaml",
	"etc/security-agent.yaml",
	"etc/system-probe.yaml",
}

var defaultFlareFolders = []string{
	"etc/confd/",
	"expvar",
	"logs",
}

var untestedFiles = []string{
	"/etc/confd/checksd/*",
	"/etc/confd/dist/*",
	"docker_inspect.log", // only within a docker container
	"local",              // when creating flare locally
	"logs/dogstatsd_info/dogstatsd-stats.log", // need to enable DSD
	"logs/jmxfetch.log",                       // need to enable JMX Fetch
	"profiles/*",                              // when creating profiles
	"remote-config-state.log",                 // if remote_configuration.enabled + correctly setup
	// TODO: all windows files
	// TODO: verify permissions.log content + files permissions
}
