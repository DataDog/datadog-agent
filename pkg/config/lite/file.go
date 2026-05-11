// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// resolveConfigPath picks the first existing datadog.yaml among the candidate
// paths. A path that does not end in .yaml/.yml is treated as a directory and
// "datadog.yaml" is appended.
func resolveConfigPath(candidates ...string) string {
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
			p = filepath.Join(p, "datadog.yaml")
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// readConfigFile loads path's raw bytes with any leading UTF-8 BOM stripped.
func readConfigFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}), nil
}
