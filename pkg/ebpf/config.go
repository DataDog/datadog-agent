// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"sync"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	spNS = "system_probe_config"
)

const (
	// AgentTmpDir is the agent's dedicated temporary directory. Every runtime path
	// system-probe writes to lives underneath it so that a single validation of this
	// one directory covers them all: it is verified to be a real, root-owned
	// directory with no non-root write access (and repaired if it is not) before any
	// of its contents are read, written or deleted.
	//
	// It is deliberately a constant. The path must stay inside the agent's own
	// subtree for that validation to be applicable, so it is not derived from
	// configuration. Keep the "datadog-agent" component in sync with
	// dedicatedDirName in pkg/ebpf/bytecode/runtime/asset.go.
	AgentTmpDir = "/var/tmp/datadog-agent"

	// KernelHeaderDownloadDir is where system-probe downloads kernel headers for
	// runtime compilation.
	//
	// This is NOT configurable. system-probe runs as root and both reuses and
	// recursively deletes the contents of this directory, so an unprivileged user
	// who can create or redirect any component of the path gains a root-write
	// primitive. Pinning it under AgentTmpDir is what guarantees the path is
	// covered by that directory's validation.
	KernelHeaderDownloadDir = AgentTmpDir + "/system-probe/kernel-headers"
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

	// KernelHeadersDownloadDir is the directory where system-probe downloads kernel
	// headers, if necessary. It is always KernelHeaderDownloadDir; the deprecated
	// system_probe_config.kernel_header_download_dir setting is ignored.
	KernelHeadersDownloadDir string

	// RuntimeCompilerOutputDir is the directory where the runtime compiler will store compiled programs.
	// This directory and every parent up to the filesystem root must be a root-owned directory that is
	// not writable by other users (a sticky, world-writable parent such as the default /var/tmp is
	// allowed). If that is not the case system-probe refuses to use the directory and skips runtime
	// compilation instead of loading objects from an untrusted location; see secureRuntimeDir.
	RuntimeCompilerOutputDir string

	// BTFOutputDir is the directory where extracted BTF files are stored
	BTFOutputDir string

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

	// RemoteConfigBTFEnabled indicates whether we can use remote config to obtain BTF
	RemoteConfigBTFEnabled bool

	// RemoteConfigBTFTimeout is how long we will wait for BTF information from remote config
	RemoteConfigBTFTimeout time.Duration

	// RemoteConfigBTFDownloadHost is the base URL host for downloading BTF from remote config
	RemoteConfigBTFDownloadHost string
}

// headerDownloadDirDeprecationOnce keeps the kernel_header_download_dir
// deprecation warning to a single line per process.
var headerDownloadDirDeprecationOnce sync.Once

// NewConfig creates a config with ebpf-related settings
func NewConfig() *Config {
	cfg := pkgconfigsetup.SystemProbe()
	sysconfig.Adjust(cfg)

	// kernel_header_download_dir is no longer honored; see KernelHeaderDownloadDir.
	// Warn instead of failing so that an existing configuration still starts, and
	// only when the user actually set it to something other than the pinned path.
	// NewConfig is called once per module, so warn only once.
	if key := sysconfig.FullKeyPath(spNS, "kernel_header_download_dir"); cfg.IsConfigured(key) &&
		cfg.GetString(key) != KernelHeaderDownloadDir {
		headerDownloadDirDeprecationOnce.Do(func() {
			log.Warnf("%s is deprecated and ignored: kernel headers are always downloaded to %s", key, KernelHeaderDownloadDir)
		})
	}

	c := &Config{
		BPFDebug:                 cfg.GetBool(sysconfig.FullKeyPath(spNS, "bpf_debug")),
		BPFDir:                   cfg.GetString(sysconfig.FullKeyPath(spNS, "bpf_dir")),
		ExcludedBPFLinuxVersions: cfg.GetStringSlice(sysconfig.FullKeyPath(spNS, "excluded_linux_versions")),
		EnableTracepoints:        cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_tracepoints")),
		ProcRoot:                 kernel.ProcFSRoot(),
		InternalTelemetryEnabled: cfg.GetBool(sysconfig.FullKeyPath(spNS, "telemetry_enabled")),

		EnableCORE:                  cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_co_re")),
		BTFPath:                     cfg.GetString(sysconfig.FullKeyPath(spNS, "btf_path")),
		BTFOutputDir:                cfg.GetString(sysconfig.FullKeyPath(spNS, "btf_output_dir")),
		RemoteConfigBTFEnabled:      cfg.GetBool(sysconfig.FullKeyPath(spNS, "remote_config_btf_enabled")),
		RemoteConfigBTFTimeout:      30 * time.Second,
		RemoteConfigBTFDownloadHost: "https://install.datadoghq.com",

		EnableRuntimeCompiler:        cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_runtime_compiler")),
		RuntimeCompilerOutputDir:     cfg.GetString(sysconfig.FullKeyPath(spNS, "runtime_compiler_output_dir")),
		EnableKernelHeaderDownload:   cfg.GetBool(sysconfig.FullKeyPath(spNS, "enable_kernel_header_download")),
		KernelHeadersDirs:            cfg.GetStringSlice(sysconfig.FullKeyPath(spNS, "kernel_header_dirs")),
		KernelHeadersDownloadDir:     KernelHeaderDownloadDir,
		AptConfigDir:                 cfg.GetString(sysconfig.FullKeyPath(spNS, "apt_config_dir")),
		YumReposDir:                  cfg.GetString(sysconfig.FullKeyPath(spNS, "yum_repos_dir")),
		ZypperReposDir:               cfg.GetString(sysconfig.FullKeyPath(spNS, "zypper_repos_dir")),
		AllowPrebuiltFallback:        cfg.GetBool(sysconfig.FullKeyPath(spNS, "allow_prebuilt_fallback")),
		AllowRuntimeCompiledFallback: cfg.GetBool(sysconfig.FullKeyPath(spNS, "allow_runtime_compiled_fallback")),

		AttachKprobesWithKprobeEventsABI: cfg.GetBool(sysconfig.FullKeyPath(spNS, "attach_kprobes_with_kprobe_events_abi")),
	}

	if !configUtils.IsRemoteConfigEnabled(pkgconfigsetup.Datadog()) {
		c.RemoteConfigBTFEnabled = false
	}
	return c
}
