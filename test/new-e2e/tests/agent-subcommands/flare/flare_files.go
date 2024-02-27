// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

// defaultFlareFiles contains all the files that are included in the flare archive by default (no need for a specific configuration option)
var defaultFlareFiles = []string{
	"config-check.log",
	"diagnose.log",
	"docker_ps.log",
	"envvars.log",
	"go-routine-dump.log",
	"health.yaml",
	"install_info",
	"metadata/inventory/host.json",
	"metadata/inventory/agent.json",
	"metadata/inventory/checks.json",
	"metadata/host.json",
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

// defaultLogFiles contains all the log files that are created with a default installation
var defaultLogFiles = []string{
	"logs/agent.log",
	"logs/process-agent.log",
	"logs/trace-agent.log",
}

// allLogFiles contains all the log files that are created when enabling all products
// TODO: add these files
//   - "logs/dogstatsd_info/dogstatsd-stats.log", // need to enable DSD
//   - "logs/jmxfetch.log",                       // need to enable JMX Fetch
var allLogFiles = []string{
	"logs/agent.log",
	"logs/process-agent.log",
	"logs/security-agent.log",
	"logs/system-probe.log",
	"logs/trace-agent.log",
}

// defaultConfigFiles contains the config file created with a default installation
var defaultConfigFiles = []string{"etc/datadog.yaml"}

// allConfigFiles contains all the config files we can create to configure the Agents
var allConfigFiles = []string{
	"etc/datadog.yaml",
	"etc/security-agent.yaml",
	"etc/system-probe.yaml",
}

// defaultFlareFolders contains all the folders included by default in the flare
var defaultFlareFolders = []string{
	"etc/confd/",
	"expvar",
	"logs",
}

// windowsFiles contains files that are specific to Windows
var windowsFiles = []string{
	"typeperf.txt",
	"lodctr.txt",
	"counter_strings.txt",
	"servicestatus.json",
	"datadog.reg",
}

// untestedFiles contains some untested files that needs specific scenario which should be added later.
//
//nolint:unused
var untestedFiles = []string{
	"docker_inspect.log",      // only within a docker container
	"local",                   // when creating flare locally
	"profiles/*",              // when creating profiles
	"remote-config-state.log", // if remote_configuration.enabled + correctly setup
	// TODO: all windows files
	// TODO: verify permissions.log content + files permissions
}
