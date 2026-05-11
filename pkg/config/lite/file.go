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
// paths the caller supplied. A path that does not end in .yaml/.yml is
// treated as a directory and "datadog.yaml" is appended.
func resolveConfigPath(candidates ...string) string {
	for _, p := range candidates {
		if p == "" {
			continue
		}
		candidate := p
		if !strings.HasSuffix(candidate, ".yaml") && !strings.HasSuffix(candidate, ".yml") {
			candidate = filepath.Join(candidate, "datadog.yaml")
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// readConfigFile loads the raw bytes from the resolved path and strips a
// UTF-8 BOM if present. Errors are returned to the caller so the rescue path
// can include them in the issue context.
func readConfigFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}), nil
}
