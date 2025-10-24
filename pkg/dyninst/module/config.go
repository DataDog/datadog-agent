// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Config is the configuration for the dynamic instrumentation module.
type Config struct {
	ebpf.Config
	LogUploaderURL     string
	DiagsUploaderURL   string
	SymDBUploadEnabled bool
	SymDBUploaderURL   string

	// DiskCacheEnabled enables the disk cache for debug info.  If this is
	// false, no disk cache will be used and the debug info will be stored in
	// memory.
	DiskCacheEnabled bool
	// DiskCacheConfig is the configuration for the disk cache for debug info.
	DiskCacheConfig object.DiskCacheConfig

	// ProcessSyncDisabled disables the process sync for the module.
	ProcessSyncDisabled bool

	TestingKnobs struct {
		LoaderOptions             []loader.Option
		IRGeneratorOverride       func(IRGenerator) IRGenerator
		ProcessSubscriberOverride func(ProcessSubscriber) ProcessSubscriber
	}
}

// erasedActuator is an erased type for an Actuator.
type erasedActuator[A Actuator[AT], AT ActuatorTenant] struct {
	a A
}

func (e *erasedActuator[A, AT]) NewTenant(name string, rt actuator.Runtime) ActuatorTenant {
	return e.a.NewTenant(name, rt)
}

func (e *erasedActuator[A, AT]) Shutdown() error {
	return e.a.Shutdown()
}

// NewConfig creates a new Config object.
func NewConfig(_ *sysconfigtypes.Config) (*Config, error) {
	traceAgentURL := getTraceAgentURL(os.Getenv)
	cacheConfig, cacheEnabled, err := getDebugInfoDiskCacheConfig()
	if err != nil {
		return nil, err
	}

	c := &Config{
		Config:             *ebpf.NewConfig(),
		LogUploaderURL:     withPath(traceAgentURL, logUploaderPath),
		DiagsUploaderURL:   withPath(traceAgentURL, diagsUploaderPath),
		SymDBUploadEnabled: pkgconfigsetup.SystemProbe().GetBool("dynamic_instrumentation.symdb_upload_enabled"),
		SymDBUploaderURL:   withPath(traceAgentURL, symdbUploaderPath),
		DiskCacheEnabled:   cacheEnabled,
		DiskCacheConfig:    cacheConfig,
	}
	return c, nil
}

const diNS = "dynamic_instrumentation"

func getDebugInfoDiskCacheConfig() (
	cacheConfig object.DiskCacheConfig, enabled bool, err error,
) {
	cfg := pkgconfigsetup.SystemProbe()
	sysconfig.Adjust(cfg)
	key := func(k string) string {
		return sysconfig.FullKeyPath(diNS, "debug_info_disk_cache", k)
	}
	getUint64 := func(k string) (uint64, error) {
		kk := key(k)
		v := cfg.GetInt64(kk)
		if v < 0 {
			return 0, fmt.Errorf("%s must be non-negative, got %d", kk, v)
		}
		return uint64(v), nil
	}

	enabled = cfg.GetBool(key("enabled"))
	cacheConfig.DirPath = cfg.GetString(key("dir"))
	maxTotalBytes, err := getUint64("max_total_bytes")
	if err != nil {
		return object.DiskCacheConfig{}, false, err
	}
	cacheConfig.MaxTotalBytes = maxTotalBytes
	requiredDiskSpaceBytes, err := getUint64("required_disk_space_bytes")
	if err != nil {
		return object.DiskCacheConfig{}, false, err
	}
	cacheConfig.RequiredDiskSpaceBytes = requiredDiskSpaceBytes
	cacheConfig.RequiredDiskSpacePercent = cfg.GetFloat64(key("required_disk_space_percent"))
	return
}

func withPath(u url.URL, path string) string {
	u.Path = path
	return u.String()
}

const (
	agentHostEnvVar  = "DD_AGENT_HOST"
	defaultAgentHost = "localhost"

	traceAgentPortEnvVar  = "DD_TRACE_AGENT_PORT"
	defaultTraceAgentPort = "8126"

	traceAgentURLEnvVar = "DD_TRACE_AGENT_URL"

	logUploaderPath   = "/debugger/v1/input"
	diagsUploaderPath = "/debugger/v1/diagnostics"
	symdbUploaderPath = "/symdb/v1/input"
)

var errSchemeRequired = fmt.Errorf("scheme is required")

// Parse the trace agent URL from the environment variables, falling back to the
// default.
//
// TODO: Support unix socket via DD_AGENT_UNIX_DOMAIN_SOCKET.
//
// This is inspired by https://github.com/DataDog/dd-trace-java/blob/76639fbb/internal-api/src/main/java/datadog/trace/api/Config.java#L1356-L1429
func getTraceAgentURL(getEnv func(string) string) url.URL {
	if traceAgentURL := getEnv(traceAgentURLEnvVar); traceAgentURL != "" {
		u, err := url.Parse(traceAgentURL)
		if err == nil && u.Scheme == "" {
			err = errSchemeRequired
		}
		if err == nil {
			return *u
		}
		log.Warnf(
			"%s is not properly configured: %v. ignoring",
			traceAgentURLEnvVar, err,
		)
	}
	host := getEnv(agentHostEnvVar)
	if host == "" {
		host = defaultAgentHost
	}
	port := getEnv(traceAgentPortEnvVar)
	if port == "" {
		port = defaultTraceAgentPort
	}
	if _, err := strconv.Atoi(port); err != nil {
		log.Warnf(
			"%s is not a valid port: %v. ignoring",
			traceAgentPortEnvVar, err,
		)
		port = defaultTraceAgentPort
	}
	return url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
	}
}
