// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"path/filepath"
	"strings"
)

// detectContentType returns one of the three content_type values the
// demoalpha-worker recognises ("yaml", "json", "redis_conf"), or "" when
// the (integration, path) combination is not safe to ship without an
// explicit override.
//
// Restrictive on purpose: the worker's parseRedisConf is a line-based
// key/value parser tailored to redis.conf — feeding it nginx.conf or
// postgresql.conf produces noise. Restricting redis_conf to
// integration=redis keeps the demo honest until per-integration parsers
// exist on the worker side.
func detectContentType(integration, path string) string {
	base := strings.ToLower(filepath.Base(path))
	switch filepath.Ext(base) {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	}
	if integration == "redis" && (base == "redis.conf" || strings.HasSuffix(base, ".conf")) {
		return "redis_conf"
	}
	return ""
}
