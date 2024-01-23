// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config defines configuration utilities for appsec
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/DataDog/appsec-internal-go/appsec"
)

const (
	enabledEnvVar        = "DD_SERVERLESS_APPSEC_ENABLED"
	tracingEnabledEnvVar = "DD_APM_TRACING_ENABLED"
)

var standalone bool

// StartOption is used to customize the AppSec configuration when invoked with appsec.Start()
type StartOption func(c *Config)

// Config is the AppSec configuration.
type Config struct {
	// Rules is the security rules loaded via the env var DD_APPSEC_RULES.
	// When not set, the builtin rules will be used.
	Rules []byte
	// WafTimeout is the maximum WAF execution time
	WafTimeout time.Duration
	// TraceRateLimit is the rate limit of AppSec traces (per second).
	TraceRateLimit uint
	// Obfuscator is the configuration for sensitive data obfuscation (in-WAF)
	Obfuscator appsec.ObfuscatorConfig
	// APISec is the configuration for API Security schema collection
	APISec appsec.APISecConfig
}

// IsEnabled returns true when appsec is enabled when the environment variable
// It also returns whether the env var is actually set in the env or not
// DD_APPSEC_ENABLED is set to true.
func IsEnabled() (enabled bool, set bool, err error) {
	enabledStr, set := os.LookupEnv(enabledEnvVar)
	if enabledStr == "" {
		return false, set, nil
	} else if enabled, err = strconv.ParseBool(enabledStr); err != nil {
		return false, set, fmt.Errorf("could not parse %s value `%s` as a boolean value", enabledEnvVar, enabledStr)
	} else {
		return enabled, set, nil
	}
}

// IsStandalone returns whether appsec is used as a standalone product (without APM tracing) or not
func IsStandalone() bool {
	panic("not called")
}

// NewConfig returns a new appsec configuration read from the environment
func NewConfig() (*Config, error) {
	panic("not called")
}

// isStandalone is reads the env and reports whether appsec runs in standalone mode
// Used at init and for testing
func isStandalone() bool {
	value, set := os.LookupEnv(tracingEnabledEnvVar)
	enabled, _ := strconv.ParseBool(value)
	return set && !enabled
}

func init() {
	standalone = isStandalone()
}
