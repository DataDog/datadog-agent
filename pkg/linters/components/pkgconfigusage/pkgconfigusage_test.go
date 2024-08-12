// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pkgconfigusage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAll(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %s", err)
	}

	originalComponentPath := componentPath
	componentPath = "comp"

	t.Cleanup(func() {
		componentPath = originalComponentPath
	})

	testdata := filepath.Join(filepath.Dir(filepath.Dir(wd)), "testdata")
	plugin := &pkgconfigUsagePlugin{}
	analyzers, err := plugin.BuildAnalyzers()
	assert.NoError(t, err)

	analyzer := analyzers[0]
	// We do this to skip issues with issues with import or other errors
	// We only care about parsing the test file and run the analyzer
	analyzer.RunDespiteErrors = true

	analysistest.Run(t, testdata, analyzers[0], "comp/...")
}
