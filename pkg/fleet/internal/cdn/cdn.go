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

// New creates a new CDN.
func New(env *env.Env, configDBPath string) (CDN, error) {
	if !env.RemotePolicies {
		return nil, nil
	}
	if env.CDNLocalDirPath != "" {
		return newLocal(env)
	}
	if !env.CDNEnabled && runtime.GOOS != "windows" {
		// Windows can't use the direct CDN yet as it breaks the static binary build
		return newDirect(env, configDBPath)
	}
	return newRegular(env, configDBPath)
}
