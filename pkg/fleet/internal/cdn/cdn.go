// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cdn provides access to the Remote Config CDN.
package cdn

import (
	"context"
	"errors"
	"regexp"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
)

const configOrderID = "configuration_order"

var (
	datadogConfigIDRegexp = regexp.MustCompile(`^datadog/\d+/AGENT_CONFIG/([^/]+)/[^/]+$`)
	// ErrProductNotSupported is returned when the product is not supported.
	ErrProductNotSupported = errors.New("product not supported")
)

type orderConfig struct {
	Order []string `json:"order"`
}

// Config represents a configuration.
type Config interface {
	Version() string
	Write(dir string) error
}

// CDN provides access to the Remote Config CDN.
type CDN interface {
	Get(ctx context.Context, pkg string) (Config, error)
	Close() error
}

// New creates a new CDN and chooses the implementation depending
// on the environment
func New(env *env.Env, configDBPath string) (CDN, error) {
	if runtime.GOOS == "windows" {
		// There's an assumption on windows that some directories are already there
		// but they are in fact created by the regular CDN implementation. Until
		// there is a fix on windows we keep the previous CDN behaviour for them
		return newCDNHTTP(env, configDBPath)
	}

	if !env.RemotePolicies {
		// Remote policies are not enabled -- we don't need the CDN
		// and we don't want to create the directories that the CDN
		// implementation would create. We return a no-op CDN to avoid
		// nil pointer dereference.
		return newCDNNoop()
	}

	if env.CDNLocalDirPath != "" {
		// Mock the CDN for local development or testing
		return newCDNLocal(env)
	}

	if !env.CDNEnabled {
		// Remote policies are enabled but we don't want to use the CDN
		// as it's still in development. We use standard remote config calls
		// instead (dubbed "direct" CDN).
		return newCDNRC(env, configDBPath)
	}

	// Regular CDN with the cloudfront distribution
	return newCDNHTTP(env, configDBPath)
}
