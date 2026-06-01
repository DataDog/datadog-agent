// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configingestion

import (
	"path/filepath"
	"strings"
)

// contentTypeUnknown is returned when no parser is registered for the file.
// The EvP worker still accepts the file; it simply skips structured parsing.
const contentTypeUnknown = ""

func detectContentType(integration, path string) string {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".conf":
		// Only redis has a registered .conf parser; other integrations are unrecognised.
		if integration == "redis" {
			return "redis_conf"
		}
	}

	return contentTypeUnknown
}
