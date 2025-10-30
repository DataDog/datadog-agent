// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package config provides configuration structures for the private action runner.
package config

import (
	"crypto/ecdsa"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// Config represents the configuration for the private action runner.
type Config struct {
	ActionsAllowlist  map[string][]string // map of allowed bundle IDs to a set of allowed action names
	Allowlist         []string
	AllowIMDSEndpoint bool
	DDHost            string
	Modes             []string
	OrgID             int64
	PrivateKey        *ecdsa.PrivateKey
	RunnerID          string
	Urn               string

	// RemoteConfig related fields
	DatadogSite string

	// the following are constants with default values. They are part of the config struct to allow for the ability to be overwritten in the YAML config file if needed
	MaxBackoff                time.Duration
	MinBackoff                time.Duration
	MaxAttempts               int32
	WaitBeforeRetry           time.Duration
	LoopInterval              time.Duration
	OpmsRequestTimeout        int32
	RunnerPoolSize            int32
	HealthCheckInterval       int32
	HTTPServerReadTimeout     int32
	HTTPServerWriteTimeout    int32
	RunnerAccessTokenHeader   string
	RunnerAccessTokenIDHeader string
	Port                      int32
	JWTRefreshInterval        time.Duration
	HealthCheckEndpoint       string
	HeartbeatInterval         time.Duration

	Version string

	MetricsClient statsd.ClientInterface
}
