// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

// DefaultFlareFiles contains all the files that are included in the flare archive by default (no need for a specific configuration option)
var DefaultFlareFiles = []string{
	"config-check.log",
	"diagnose.log",
	"docker_ps.log",
	"envvars.log",
	"go-routine-dump.log",
	"health.yaml",
	"install_info.log",
	"permissions.log",
	"process_agent_runtime_config_dump.yaml",
	"runtime_config_dump.yaml",
	"secrets.log",
	"status.log",
	"system_probe_runtime_config_dump.yaml",
	"version-history.json",
}

// DefaultMetadataFlareFiles contains the metadata files included by default
var DefaultMetadataFlareFiles = []string{
	"metadata/host.json",
	"metadata/inventory/host.json",
	"metadata/inventory/agent.json",
}

// NonLocalMetadataFlareFiles contains metadata files only available in non-local flare mode
var NonLocalMetadataFlareFiles = []string{
	"metadata/inventory/checks.json",
}

// NonLocalFlareFiles contains files only available in non-local flare mode
var NonLocalFlareFiles = []string{
	"tagger-list.json",
	"workload-list.log",
	"agent_open_files.txt",
}

// DefaultLogFiles contains all the log files that are created with a default installation
var DefaultLogFiles = []string{
	"logs/agent.log",
	"logs/process-agent.log",
	"logs/trace-agent.log",
}

// AllLogFiles contains all the log files that are created when enabling all products
// TODO: add these files
//   - "logs/dogstatsd_info/dogstatsd-stats.log", // need to enable DSD
//   - "logs/jmxfetch.log",                       // need to enable JMX Fetch
var AllLogFiles = []string{
	"logs/agent.log",
	"logs/process-agent.log",
	"logs/security-agent.log",
	"logs/system-probe.log",
	"logs/trace-agent.log",
}

// DefaultConfigFiles contains the config file created with a default installation
var DefaultConfigFiles = []string{"etc/datadog.yaml"}

// AllConfigFiles contains all the config files we can create to configure the Agents
var AllConfigFiles = []string{
	"etc/datadog.yaml",
	"etc/security-agent.yaml",
	"etc/system-probe.yaml",
}

// DefaultFlareFolders contains all the folders included by default in the flare
var DefaultFlareFolders = []string{
	"etc/confd/",
	"expvar",
	"logs",
}

// WindowsFiles contains files that are specific to Windows
var WindowsFiles = []string{
	"typeperf.txt",
	"lodctr.txt",
	"counter_strings.txt",
	"servicestatus.json",
	"datadog.reg",
}

// LinuxFiles contains files that are specific to Linux
var LinuxFiles = []string{}

// ProfilingFiles contains the profiling files included in a flare with profiling enabled
var ProfilingFiles = []string{
	"profiles/core-1st-heap.pprof",
	"profiles/core-2nd-heap.pprof",
	"profiles/core-block.pprof",
	"profiles/core-cpu.pprof",
	"profiles/core-mutex.pprof",
	"profiles/trace-1st-heap.pprof",
	"profiles/trace-2nd-heap.pprof",
	"profiles/trace-block.pprof",
	"profiles/trace-cpu.pprof",
	"profiles/trace-mutex.pprof",
}

// ProfilingNonLinuxFiles contains profiling files only available on non-Linux platforms
var ProfilingNonLinuxFiles = []string{
	"profiles/process-1st-heap.pprof",
	"profiles/process-2nd-heap.pprof",
	"profiles/process-block.pprof",
	"profiles/process-cpu.pprof",
	"profiles/process-mutex.pprof",
}

// UntestedFiles contains some untested files that needs specific scenario which should be added later.
//
//nolint:unused
var UntestedFiles = []string{
	"docker_inspect.log",      // only within a docker container
	"local",                   // when creating flare locally
	"profiles/*",              // when creating profiles
	"remote-config-state.log", // if remote_configuration.enabled + correctly setup
	// TODO: all windows files
	// TODO: verify permissions.log content + files permissions
}
