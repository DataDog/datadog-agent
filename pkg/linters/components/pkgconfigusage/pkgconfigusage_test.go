// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pkgconfigusage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/testutil"
	"github.com/OpenPeeDeeP/depguard/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"gopkg.in/yaml.v3"
)

func TestAll(t *testing.T) {
	for key, value := range testutil.IsolatedGoBuildEnv(t.TempDir()) {
		t.Setenv(key, value)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %s", err)
	}

	testdata := filepath.Join(filepath.Dir(filepath.Dir(wd)), "testdata")
	analyzer := pkgconfigusageAnalyzer(t, wd)
	// We do this to skip issues with import or other errors.
	// We only care about parsing the test file and run the analyzer.
	analyzer.RunDespiteErrors = true

	analysistest.Run(t, testdata, analyzer, "./...")
}

func pkgconfigusageAnalyzer(t *testing.T, wd string) *analysis.Analyzer {
	configData, err := os.ReadFile(filepath.Join(wd, "..", "..", "..", "..", ".golangci.yml"))
	require.NoError(t, err)

	var cfg struct {
		Linters struct {
			Settings struct {
				Depguard struct {
					Rules map[string]struct {
						Files []string
						Deny  []struct{ Pkg, Desc string }
					}
				}
			}
		}
	}
	require.NoError(t, yaml.Unmarshal(configData, &cfg))

	rule, ok := cfg.Linters.Settings.Depguard.Rules["pkgconfigusage"]
	require.True(t, ok, "pkgconfigusage rule must exist in .golangci.yml")

	deny := make(map[string]string, len(rule.Deny))
	for _, d := range rule.Deny {
		deny[d.Pkg] = d.Desc
	}
	analyzer, err := depguard.NewAnalyzer(&depguard.LinterSettings{
		"pkgconfigusage": &depguard.List{Files: rule.Files, Deny: deny},
	})
	require.NoError(t, err)
	return analyzer
}
