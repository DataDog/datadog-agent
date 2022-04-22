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

	// NetworkConfig is the network tracer configuration
	NetworkConfig netconfig.Config

	// SecurityConfig is the runtime security configuration
	// This is needed to determine which runtime-compilable security features have been enabled
	SecurityConfig secconfig.Config

	// EnableNetworkCompilation enables the runtime compilation of network assets
	// This is needed to determine which runtime-compilable networks features have been enabled
	EnableNetworkCompilation bool

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
}

// NewConfig creates a config for the runtime compiler
func NewConfig(sysprobeconfig *config.Config) *Config {
	cfg := ddconfig.Datadog
	ddconfig.InitSystemProbeConfig(cfg)

	netConfig := *netconfig.New()
	secConfig := *secconfig.NewConfig(sysprobeconfig)

	return &Config{
		Config:         *ebpf.NewConfig(),
		NetworkConfig:  netConfig,
		SecurityConfig: secConfig,

		EnableNetworkCompilation:         cfg.GetBool(key(nwNS, "enabled")) && netConfig.EnableRuntimeCompilation,
		EnableRuntimeSecurityCompilation: cfg.GetBool(key(rsNS, "enabled")) && secConfig.RuntimeCompilationEnabled,
		EnableConstantFetcherCompilation: cfg.GetBool(key(rsNS, "enabled")) && secConfig.RuntimeCompiledConstantsEnabled,
		EnableTcpQueueLengthCompilation:  cfg.GetBool(key(spNS, "enable_tcp_queue_length")),
		EnableOomKillCompilation:         cfg.GetBool(key(spNS, "enable_oom_kill")),
	}
}

func key(pieces ...string) string {
	return strings.Join(pieces, ".")
}
