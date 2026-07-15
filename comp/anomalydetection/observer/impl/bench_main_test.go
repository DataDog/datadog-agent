// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"flag"
	"os"
	"strings"
	"testing"
)

// benchmarkSettings is the ComponentSettings used by all benchmarks.
// Defaults to empty (uses catalog defaults). Override with -only flag:
//
//	go test -bench=. -args -only=bocpd,time_cluster
var benchmarkSettings ComponentSettings

func TestMain(m *testing.M) {
	onlyStr := flag.String("only", "", "Enable ONLY these components (plus extractors); disable everything else.")
	flag.Parse()

	if *onlyStr != "" {
		onlySet := make(map[string]bool)
		for _, name := range strings.Split(*onlyStr, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				onlySet[name] = true
			}
		}
		overrides := make(map[string]bool)
		for _, entry := range TestbenchCatalogEntries() {
			if entry.Kind == "extractor" {
				continue // extractors always enabled
			}
			overrides[entry.Name] = onlySet[entry.Name]
		}
		benchmarkSettings = ComponentSettings{Enabled: overrides}
	}

	os.Exit(m.Run())
}
