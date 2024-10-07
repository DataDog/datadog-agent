// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package funcguard provides is a linter to detect unwanted/deprecated function calls in the code base.
package funcguard

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

	testdata := filepath.Join(wd, "testdata")
	plugin := &forbiddenUsagePlugin{
		rules: map[string]string{
			"github.com/DataDog/datadog-agent/pkg/config/model.NewConfig": "error detected on method call",
			"fmt.Printf": "Printf detected",
		},
	}
	analyzers, err := plugin.BuildAnalyzers()
	assert.NoError(t, err)

	analyzer := analyzers[0]
	// We do this to skip issues with import or other errors.
	// We only care about parsing the test file and run the analyzer.
	analyzer.RunDespiteErrors = true

	analysistest.Run(t, testdata, analyzer, "")
}
