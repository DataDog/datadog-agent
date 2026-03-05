// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"crypto/ecdsa"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/gobwas/glob"
	"k8s.io/apimachinery/pkg/util/sets"
)

type Config struct {
	ActionsAllowlist  map[string]sets.Set[string] // map of allowed bundle IDs to a set of allowed action names
	Allowlist         []string
	AllowIMDSEndpoint bool
	DDHost            string
	DDApiHost         string
	Modes             []modes.Mode
	OrgId             int64
	PrivateKey        *ecdsa.PrivateKey
	RunnerId          string
	Urn               string
	Tags              []observability.Tag

	// RemoteConfig related fields
	DatadogSite string

	// the following are constants with default values. They are part of the config struct to allow for the ability to be overwritten in the YAML config file if needed
	MaxBackoff                 time.Duration
	MinBackoff                 time.Duration
	MaxAttempts                int32
	WaitBeforeRetry            time.Duration
	LoopInterval               time.Duration
	OpmsRequestTimeout         int32
	RunnerPoolSize             int32
	HealthCheckInterval        int32
	HttpServerReadTimeout      int32
	HttpServerWriteTimeout     int32
	HTTPTimeout                time.Duration
	TaskTimeoutSeconds         *int32
	RunnerAccessTokenHeader    string
	RunnerAccessTokenIdHeader  string
	Port                       int32
	JWTRefreshInterval         time.Duration
	HealthCheckEndpoint        string
	HeartbeatInterval          time.Duration
	EnableProfiling            bool
	DisableCredentialTemplates bool

	Version string

	MetricsClient statsd.ClientInterface
}

func (c *Config) IsActionAllowed(bundleId, actionName string) bool {
	if _, ok := c.ActionsAllowlist[bundleId]; ok {
		return c.ActionsAllowlist[bundleId].HasAny(actionName, "*")
	}
	return false
}

func (c *Config) IsURLInAllowlist(urlStr string) bool {
	if c.Allowlist == nil {
		return true
	}

	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		fmt.Printf("Invalid URL: %v\n", err)
		return false
	}
	hostname := parsedURL.Hostname()

	// Convert hostname to lowercase for case-insensitive comparison
	hostname = strings.ToLower(hostname)

	// Check for exact matches
	for _, pattern := range c.Allowlist {
		if strings.ToLower(pattern) == hostname {
			return true
		}
	}
	// Check for glob pattern matches
	for _, pattern := range c.Allowlist {
		globPattern := strings.ToLower(pattern)
		matcher := glob.MustCompile(globPattern)
		if matcher.Match(hostname) {
			return true
		}
	}

	return false
}

func (c *Config) IdentityIsIncomplete() bool {
	return c.Urn == "" || c.PrivateKey == nil
}
