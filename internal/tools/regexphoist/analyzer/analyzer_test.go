// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package analyzer_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/DataDog/datadog-agent/internal/tools/regexphoist/analyzer"
)

func TestAnalyzer(t *testing.T) {
	if rootFile := os.Getenv("REGEXPHOIST_GO_SDK_ROOT_FILE"); rootFile != "" {
		goroot := filepath.Dir(filepath.Join(os.Getenv("TEST_SRCDIR"), rootFile))
		t.Setenv("GOROOT", goroot)
		t.Setenv("PATH", filepath.Join(goroot, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv("GOTOOLCHAIN", "local")
		t.Setenv("GOPACKAGESDRIVER", "off")
		t.Setenv("GOCACHE", t.TempDir())
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("analysistest requires the Go SDK: %v", err)
	}
	analysistest.Run(t, analysistest.TestData(), analyzer.Analyzer, "example")
}
