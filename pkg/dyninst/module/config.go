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

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Config is the configuration for the dynamic instrumentation module.
type Config struct {
	ebpf.Config
	DynamicInstrumentationEnabled bool
	LogUploaderURL                string
	DiagsUploaderURL              string

	actuatorConstructor erasedActuatorConstructor
}

// Option is an option that can be passed to NewConfig.
type Option interface {
	apply(c *Config)
}

type wrappedActuator[A Actuator[T], T ActuatorTenant] struct {
	actuator A
}

func eraseActuator[A Actuator[T], T ActuatorTenant](a A) erasedActuator {
	return wrappedActuator[A, T]{actuator: a}
}

func (a wrappedActuator[A, T]) Shutdown() error {
	return a.actuator.Shutdown()
}

func (a wrappedActuator[A, T]) NewTenant(
	name string,
	reporter actuator.Reporter,
	irGenerator actuator.IRGenerator,
) ActuatorTenant {
	return a.actuator.NewTenant(name, reporter, irGenerator)
}

type erasedActuator = Actuator[ActuatorTenant]

type actuatorConstructor[A Actuator[T], T ActuatorTenant] func(*loader.Loader) A
type erasedActuatorConstructor = actuatorConstructor[erasedActuator, ActuatorTenant]

func (a actuatorConstructor[A, T]) apply(c *Config) {
	c.actuatorConstructor = func(t *loader.Loader) erasedActuator {
		return eraseActuator(a(t))
	}
}

// WithActuatorConstructor is an option that allows the user to provide a
func WithActuatorConstructor[
	A Actuator[T], T ActuatorTenant,
](
	f actuatorConstructor[A, T],
) Option {
	return actuatorConstructor[A, T](f)
}

func defaultActuatorConstructor(t *loader.Loader) erasedActuator {
	return eraseActuator(actuator.NewActuator(t))
}

// NewConfig creates a new Config object
func NewConfig(spConfig *sysconfigtypes.Config, opts ...Option) (*Config, error) {
	var diEnabled bool
	if spConfig != nil {
		_, diEnabled = spConfig.EnabledModules[sysconfig.DynamicInstrumentationModule]
	}
	traceAgentURL := getTraceAgentURL(os.Getenv)
	c := &Config{
		Config:                        *ebpf.NewConfig(),
		DynamicInstrumentationEnabled: diEnabled,
		LogUploaderURL:                withPath(traceAgentURL, logUploaderPath),
		DiagsUploaderURL:              withPath(traceAgentURL, diagsUploaderPath),
		actuatorConstructor:           defaultActuatorConstructor,
	}
	for _, opt := range opts {
		opt.apply(c)
	}
	return c, nil
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
