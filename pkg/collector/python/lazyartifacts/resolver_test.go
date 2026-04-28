// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lazyartifacts

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePlatform(t *testing.T) {
	platform, err := parsePlatform("linux/arm64")
	if err != nil {
		t.Fatalf("parsePlatform returned an error: %v", err)
	}
	if platform.OS != "linux" || platform.Architecture != "arm64" {
		t.Fatalf("unexpected platform: %+v", platform)
	}

	if _, err := parsePlatform("linux"); err == nil {
		t.Fatal("expected invalid platform to return an error")
	}
}

func TestPythonCheckImportPath(t *testing.T) {
	cacheDir := filepath.Join("cache", "python-check-mongo")
	checkPath := "opt/datadog-agent/embedded/lib/python3.13/site-packages/datadog_checks/mongo"

	importPath, err := pythonCheckImportPath(cacheDir, checkPath, "mongo")
	if err != nil {
		t.Fatalf("pythonCheckImportPath returned an error: %v", err)
	}

	expected := filepath.Join(cacheDir, "root", "opt", "datadog-agent", "embedded", "lib", "python3.13", "site-packages")
	if importPath != expected {
		t.Fatalf("unexpected import path: got %q want %q", importPath, expected)
	}
}

func TestPythonCheckImportPathRejectsUnexpectedPath(t *testing.T) {
	_, err := pythonCheckImportPath("cache", "opt/datadog-agent/embedded/lib/python3.13/site-packages/foo/mongo", "mongo")
	if err == nil {
		t.Fatal("expected invalid check path to return an error")
	}
}

func TestCacheKey(t *testing.T) {
	key := cacheKey("sha256:abc", "python-check:mongo")
	if strings.ContainsAny(key, "/:") {
		t.Fatalf("cache key contains a path separator or colon: %q", key)
	}
	if !strings.HasPrefix(key, "python-check_mongo-") {
		t.Fatalf("unexpected cache key prefix: %q", key)
	}
}
