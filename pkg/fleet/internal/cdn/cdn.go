// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cdn provides access to the Remote Config CDN.
package cdn

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// CDN provides access to the Remote Config CDN.
type CDN struct {
	env *env.Env
}

// Config represents the configuration from the CDN.
type Config struct {
	Version       string
	Datadog       []byte
	SecurityAgent []byte
	SystemProbe   []byte
}

// New creates a new CDN.
func New(env *env.Env) *CDN {
	return &CDN{
		env: env,
	}
}

// Get gets the configuration from the CDN.
func (c *CDN) Get(ctx context.Context) (_ Config, err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "cdn.Get")
	defer func() { span.Finish(tracer.WithError(err)) }()
	return getFakeCDNConfig(ctx)
}

// HACK (arthur): this is a temporary function that returns a fake CDN config to unblock development.
func getFakeCDNConfig(_ context.Context) (Config, error) {
	baseLayer := layer{
		ID: "base",
		Content: map[interface{}]interface{}{
			"extra_tags": []string{"layer:base"},
		},
	}
	overrideLayer := layer{
		ID: "override",
		Content: map[interface{}]interface{}{
			"extra_tags": []string{"layer:override"},
		},
	}
	config, err := newConfig(baseLayer, overrideLayer)
	if err != nil {
		return Config{}, err
	}
	serializedConfig, err := config.Marshal()
	if err != nil {
		return Config{}, err
	}
	hash := sha256.New()
	hash.Write(serializedConfig)
	return Config{
		Version: fmt.Sprintf("%x", hash.Sum(nil)),
		Datadog: serializedConfig,
	}, nil
}
