// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package imageresolver provides configuration and utilities for resolving
// container image references from mutable tags to digests.
package imageresolver

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

const (
	rolloutBucketCount = 10 // Max number of buckets for gradual rollout
)

// Config contains information needed to create a Resolver
type Config struct {
	Site           string
	DDRegistries   map[string]struct{}
	BucketID       string
	DigestCacheTTL time.Duration
	Enabled        bool
}

func calculateRolloutBucket(apiKey string) string {
	// DEV: If the API key is empty for whatever reason, resolves to bucket 2
	hash := sha256.Sum256([]byte(apiKey))
	hashInt := binary.BigEndian.Uint64(hash[:8])
	return strconv.Itoa(int(hashInt % rolloutBucketCount))
}

func NewConfig(cfg config.Component) Config {
	return Config{
		Site:           cfg.GetString("site"),
		DDRegistries:   newDatadoghqRegistries(cfg.GetStringSlice("admission_controller.auto_instrumentation.default_dd_registries")),
		BucketID:       calculateRolloutBucket(cfg.GetString("api_key")),
		DigestCacheTTL: cfg.GetDuration("admission_controller.auto_instrumentation.gradual_rollout.cache_ttl"),
		Enabled:        cfg.GetBool("admission_controller.auto_instrumentation.gradual_rollout.enabled"),
	}
}
