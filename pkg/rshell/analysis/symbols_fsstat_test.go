// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analysis

import "testing"

func fsstatCheckConfig() allowedSymbolsConfig {
	return allowedSymbolsConfig{
		Symbols:      fsstatAllowedSymbols,
		TargetDir:    "allowedpaths/internal/fsstat",
		CollectFiles: collectFlatGoFiles,
		ListName:     "fsstatAllowedSymbols",
		// The package has one common file and one backend for each supported
		// platform: Linux, Darwin, and Windows.
		MinFiles: 4,
	}
}

func TestFSStatAllowedSymbols(t *testing.T) {
	checkAllowedSymbols(t, fsstatCheckConfig())
}
