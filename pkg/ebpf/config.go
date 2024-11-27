// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const (
	spNS = "system_probe_config"
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

	// InternalTelemetryEnabled indicates whether internal prometheus telemetry is enabled
	InternalTelemetryEnabled bool

	// EnableTracepoints enables use of tracepoints instead of kprobes for probing syscalls (if available on system)
	EnableTracepoints bool

	// EnableCORE enables the use of CO-RE to load eBPF programs
	EnableCORE bool

	// BTFPath is the path to BTF data for the current kernel
	BTFPath string

	// EnableRuntimeCompiler enables the use of the embedded compiler to build eBPF programs on-host
	EnableRuntimeCompiler bool

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

	// AllowPrebuiltFallback indicates whether we are allowed to fallback to the prebuilt probes if runtime compilation fails.
	AllowPrebuiltFallback bool

	// AllowRuntimeCompiledFallback indicates whether we are allowed to fallback to runtime compilation if CO-RE fails.
	AllowRuntimeCompiledFallback bool

	// AttachKprobesWithKprobeEventsABI uses the kprobe_events ABI to attach kprobes rather than the newer perf ABI.
	AttachKprobesWithKprobeEventsABI bool

	// BypassEnabled is used in tests only.
	// It enables a ebpf-manager feature to bypass programs on-demand for controlled visibility.
	BypassEnabled bool
}

// NewConfig creates a config with ebpf-related settings
func NewConfig() *Config {
	cfg := pkgconfigsetup.SystemProbe()
	sysconfig.Adjust(cfg)

	c := &Config{
		BPFDebug:                 cfg.GetBool(sysconfig.FullKeyPath(spNS, "bpf_debug")),
		BPFDir:                   cfg.GetString(sysconfig.FullKeyPath(spNS, "bpf_dir")),
		ExcludedBPFLinuxVersions: cfg.GetStringSlice(sysconfig.FullKeyPath(spNS, "excluded_linux_versions")),
		EnableTracepoints:        cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_tracepoints")),
		ProcRoot:                 kernel.ProcFSRoot(),
		InternalTelemetryEnabled: cfg.GetBool(sysconfig.FullKeyPath(spNS, "telemetry_enabled")),

		EnableCORE: cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_co_re")),
		BTFPath:    cfg.GetString(sysconfig.FullKeyPath(spNS, "btf_path")),

		EnableRuntimeCompiler:        cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_runtime_compiler")),
		RuntimeCompilerOutputDir:     cfg.GetString(sysconfig.FullKeyPath(spNS, "runtime_compiler_output_dir")),
		EnableKernelHeaderDownload:   cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_kernel_header_download")),
		KernelHeadersDirs:            cfg.GetStringSlice(sysconfig.FullKeyPath(spNS, "kernel_header_dirs")),
		KernelHeadersDownloadDir:     cfg.GetString(sysconfig.FullKeyPath(spNS, "kernel_header_download_dir")),
		AptConfigDir:                 cfg.GetString(sysconfig.FullKeyPath(spNS, "apt_config_dir")),
		YumReposDir:                  cfg.GetString(sysconfig.FullKeyPath(spNS, "yum_repos_dir")),
		ZypperReposDir:               cfg.GetString(sysconfig.FullKeyPath(spNS, "zypper_repos_dir")),
		AllowPrebuiltFallback:        cfg.GetBool(sysconfig.FullKeyPath(spNS, "allow_prebuilt_fallback")),
		AllowRuntimeCompiledFallback: cfg.GetBool(sysconfig.FullKeyPath(spNS, "allow_runtime_compiled_fallback")),

		AttachKprobesWithKprobeEventsABI: cfg.GetBool(sysconfig.FullKeyPath(spNS, "attach_kprobes_with_kprobe_events_abi")),
	}

	return c
}
