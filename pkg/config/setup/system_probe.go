// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"os"
	"path"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	defaultConnsMessageBatchSize = 600

	// defaultRuntimeCompilerOutputDir is the default path for output from the system-probe runtime compiler
	defaultRuntimeCompilerOutputDir = "/var/tmp/datadog-agent/system-probe/build"

	// defaultKernelHeadersDownloadDir is the default path for downloading kernel headers for runtime compilation
	defaultKernelHeadersDownloadDir = "/var/tmp/datadog-agent/system-probe/kernel-headers"

	// defaultBTFOutputDir is the default path for extracted BTF
	defaultBTFOutputDir = "/var/tmp/datadog-agent/system-probe/btf"

	// defaultDynamicInstrumentationDebugInfoDir is the default path for debug
	// info for Dynamic Instrumentation. This is the directory where the DWARF
	// data from analyzed binaries is decompressed into during processing.
	defaultDynamicInstrumentationDebugInfoDir = "${run_path}/system-probe/dynamic-instrumentation/decompressed-debug-info"

	// defaultAptConfigDirSuffix is the default path under `/etc` to the apt config directory
	defaultAptConfigDirSuffix = "/apt"

	// defaultYumReposDirSuffix is the default path under `/etc` to the yum repository directory
	defaultYumReposDirSuffix = "/yum.repos.d"

	// defaultZypperReposDirSuffix is the default path under `/etc` to the zypper repository directory
	defaultZypperReposDirSuffix = "/zypp/repos.d"

	defaultOffsetThreshold = 400

	// defaultEnvoyPath is the default path for envoy binary
	defaultEnvoyPath = "/bin/envoy"
)

// InitSystemProbeConfig declares all the configuration values normally read from system-probe.yaml.
func InitSystemProbeConfig(cfg pkgconfigmodel.Setup) {
	initMainSystemProbeConfig(cfg)
	initCWSSystemProbeConfig(cfg)
	initUSMSystemProbeConfig(cfg)
}

func suffixHostEtc(suffix string) string {
	if value, _ := os.LookupEnv("HOST_ETC"); value != "" {
		return path.Join(value, suffix)
	}
	return path.Join("/etc", suffix)
}

// eventMonitorBindEnvAndSetDefault is a helper function that generates both "DD_RUNTIME_SECURITY_CONFIG_" and "DD_EVENT_MONITORING_CONFIG_"
// prefixes from a key. We need this helper function because the standard BindEnvAndSetDefault can only generate one prefix, but we want to
// support both for backwards compatibility.
func eventMonitorBindEnvAndSetDefault(config pkgconfigmodel.Setup, key string, val interface{}) {
	// Uppercase, replace "." with "_" and add "DD_" prefix to key so that we follow the same environment
	// variable convention as the core agent.
	emConfigKey := "DD_" + strings.ReplaceAll(strings.ToUpper(key), ".", "_")
	runtimeSecKey := strings.Replace(emConfigKey, "EVENT_MONITORING_CONFIG", "RUNTIME_SECURITY_CONFIG", 1)

	config.BindEnvAndSetDefault(key, val, emConfigKey, runtimeSecKey)
}

// DefaultPrivateIPCIDRs is a list of private IP CIDRs that are used to determine if an IP is private or not.
var DefaultPrivateIPCIDRs = []string{
	// IETF RPC 1918
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	// IETF RFC 5735
	"0.0.0.0/8",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	// IETF RFC 6598
	"100.64.0.0/10",
	// IETF RFC 4193
	"fc00::/7",
	// IPv6 loopback address
	"::1/128",
}
