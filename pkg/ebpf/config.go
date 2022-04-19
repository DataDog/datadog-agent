// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"strings"

	aconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	spNS = "system_probe_config"
	rcNS = "runtime_compiler_config"
)

// Config stores all common flags used by system-probe
type Config struct {
	// BPFDebug enables bpf debug logs
	BPFDebug bool

	// BPFDir is the directory to load the eBPF program from
	BPFDir string

	// ExcludedBPFLinuxVersions lists Linux kernel versions that should not use BPF features
	ExcludedBPFLinuxVersions []string

	// ProcRoot is the root path to the proc filesystem
	ProcRoot string

	// EnableTracepoints enables use of tracepoints instead of kprobes for probing syscalls (if available on system)
	EnableTracepoints bool

	// EnableKernelHeaderDownload enables the use of the automatic kernel header downloading
	EnableKernelHeaderDownload bool

	// KernelHeadersDir is the directories of the kernel headers to use for runtime compilation
	KernelHeadersDirs []string

	// KernelHeadersDownloadDir is the directory where the system-probe will attempt to download kernel headers, if necessary
	KernelHeadersDownloadDir string

	// RuntimeCompilerOutputDir is the directory where the runtime compiler will store compiled programs
	RuntimeCompilerOutputDir string

	// AptConfigDir is the path to the apt config directory
	AptConfigDir string

	// YumReposDir is the path to the yum repository directory
	YumReposDir string

	// ZypperReposDir is the path to the zypper repository directory
	ZypperReposDir string
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}

// NewConfig creates a config with ebpf-related settings
func NewConfig() *Config {
	cfg := aconfig.Datadog
	aconfig.InitSystemProbeConfig(cfg)

	return &Config{
		BPFDebug:                   cfg.GetBool(key(spNS, "bpf_debug")),
		BPFDir:                     cfg.GetString(key(spNS, "bpf_dir")),
		ExcludedBPFLinuxVersions:   cfg.GetStringSlice(key(spNS, "excluded_linux_versions")),
		EnableTracepoints:          cfg.GetBool(key(spNS, "enable_tracepoints")),
		ProcRoot:                   util.GetProcRoot(),
		RuntimeCompilerOutputDir:   cfg.GetString(key(rcNS, "runtime_compiler_output_dir")),
		EnableKernelHeaderDownload: cfg.GetBool(key(rcNS, "enable_kernel_header_download")),
		KernelHeadersDirs:          cfg.GetStringSlice(key(rcNS, "kernel_header_dirs")),
		KernelHeadersDownloadDir:   cfg.GetString(key(rcNS, "kernel_header_download_dir")),
		AptConfigDir:               cfg.GetString(key(rcNS, "apt_config_dir")),
		YumReposDir:                cfg.GetString(key(rcNS, "yum_repos_dir")),
		ZypperReposDir:             cfg.GetString(key(rcNS, "zypper_repos_dir")),
	}
}
