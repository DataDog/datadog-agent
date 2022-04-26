// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package config

import (
	"strings"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
)

const (
	rcNS = "runtime_compiler_config"
	spNS = "system_probe_config"
	nwNS = "network_config"
	rsNS = "runtime_security_config"
)

// Config stores all flags related to runtime compilation
type Config struct {
	ebpf.Config

	// EnableNetworkCompilation enables the runtime compilation of network assets
	EnableNetworkCompilation bool

	// ConntrackEnabled indicates whether the ebpf conntracker (a network asset) has been enabled
	ConntrackEnabled bool

	// HTTPMonitoringEnabled indicates whether the http tracer (a network asset) has been enabled
	HTTPMonitoringEnabled bool

	// CollectIPv6Conns is a network setting which we use to determine what cflags to use when compiling network assets
	CollectIPv6Conns bool

	// EnableRuntimeSecurityCompilation enables the runtime compilation of runtime security assets
	EnableRuntimeSecurityCompilation bool

	// EnableConstantFetcherCompilation enables the compilation of the runtime security constant fetcher
	//
	// Even though the constant fetcher is runtime security asset, whether or not it should be compiled
	// is independent of the other runtime security assets. This is because it is possible for
	// runtime compilation to be disabled in the runtime security probe, while still being enabled
	// for the constant fetcher. See how RuntimeCompiledConstantsEnabled is determined in the security
	// config for more details.
	EnableConstantFetcherCompilation bool

	// EnableTcpQueueLengthCompilation enables the compilation of the tcp queue length check
	EnableTcpQueueLengthCompilation bool

	// EnableOomKillCompilation enables the compilation of the oom kill check
	EnableOomKillCompilation bool

	// StatsdAddr defines the statsd address
	StatsdAddr string

	// EnableKernelHeaderDownload enables the use of the automatic kernel header downloading
	EnableKernelHeaderDownload bool

	// KernelHeadersDir is the directories of the kernel headers to use for runtime compilation
	KernelHeadersDirs []string

	// KernelHeadersDownloadDir is the directory where the system-probe will attempt to download kernel headers, if necessary
	KernelHeadersDownloadDir string

	// AptConfigDir is the path to the apt config directory
	AptConfigDir string

	// YumReposDir is the path to the yum repository directory
	YumReposDir string

	// ZypperReposDir is the path to the zypper repository directory
	ZypperReposDir string
}

// NewConfig creates a config for the runtime compiler
func NewConfig(sysprobeconfig *config.Config) *Config {
	cfg := ddconfig.Datadog
	ddconfig.InitSystemProbeConfig(cfg)

	netConfig := *netconfig.New()
	secConfig := *secconfig.NewConfig(sysprobeconfig)

	return &Config{
		Config: *ebpf.NewConfig(),

		EnableNetworkCompilation:         cfg.GetBool(key(nwNS, "enabled")) && netConfig.EnableRuntimeCompilation,
		ConntrackEnabled:                 netConfig.EnableConntrack,
		HTTPMonitoringEnabled:            netConfig.EnableHTTPMonitoring,
		CollectIPv6Conns:                 netConfig.CollectIPv6Conns,
		EnableRuntimeSecurityCompilation: cfg.GetBool(key(rsNS, "enabled")) && secConfig.RuntimeCompilationEnabled,
		EnableConstantFetcherCompilation: cfg.GetBool(key(rsNS, "enabled")) && secConfig.RuntimeCompiledConstantsEnabled,
		EnableTcpQueueLengthCompilation:  cfg.GetBool(key(spNS, "enable_tcp_queue_length")),
		EnableOomKillCompilation:         cfg.GetBool(key(spNS, "enable_oom_kill")),

		StatsdAddr: secConfig.StatsdAddr,

		// Kernel header downloading settings
		EnableKernelHeaderDownload: cfg.GetBool(key(rcNS, "enable_kernel_header_download")),
		KernelHeadersDirs:          cfg.GetStringSlice(key(rcNS, "kernel_header_dirs")),
		KernelHeadersDownloadDir:   cfg.GetString(key(rcNS, "kernel_header_download_dir")),
		AptConfigDir:               cfg.GetString(key(rcNS, "apt_config_dir")),
		YumReposDir:                cfg.GetString(key(rcNS, "yum_repos_dir")),
		ZypperReposDir:             cfg.GetString(key(rcNS, "zypper_repos_dir")),
	}
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}
