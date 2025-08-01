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

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Config is the configuration for the dynamic instrumentation module.
type Config struct {
	ebpf.Config
	DynamicInstrumentationEnabled bool
	LogUploaderURL                string
	DiagsUploaderURL              string
}

// NewConfig creates a new Config object
func NewConfig(spConfig *sysconfigtypes.Config) (*Config, error) {
	var diEnabled bool
	if spConfig != nil {
		_, diEnabled = spConfig.EnabledModules[config.DynamicInstrumentationModule]
	}
	traceAgentURL := getTraceAgentURL(os.Getenv)
	return &Config{
		Config:                        *ebpf.NewConfig(),
		DynamicInstrumentationEnabled: diEnabled,
		LogUploaderURL:                withPath(traceAgentURL, logUploaderPath),
		DiagsUploaderURL:              withPath(traceAgentURL, diagsUploaderPath),
	}, nil
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

	traceAgentURLEnvVar  = "DD_TRACE_AGENT_URL"
	defaultTraceAgentURL = "http://" + defaultAgentHost + ":" + defaultTraceAgentPort

	logUploaderPath   = "/debugger/v1/input"
	diagsUploaderPath = "/debugger/v1/diagnostics"
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
