// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cdn provides access to the Remote Config CDN.
package cdn

import (
	"context"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
)

const configOrderID = "configuration_order"

var datadogConfigIDRegexp = regexp.MustCompile(`^datadog/\d+/AGENT_CONFIG/([^/]+)/[^/]+$`)

type orderConfig struct {
	Order []string `json:"order"`
}

// CDN provides access to the Remote Config CDN.
type CDN interface {
	Get(ctx context.Context) (*Config, error)
	Close() error
}

// New creates a new CDN.
func New(env *env.Env, configDBPath string) (CDN, error) {
	if env.CDNLocalDirPath != "" {
		return newLocal(env)
	}
	return newRemote(env, configDBPath)
}
