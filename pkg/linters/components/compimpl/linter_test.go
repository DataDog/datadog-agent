// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestValid(t *testing.T) {
	runLinter(t,
		`package component
		func Module() fxutil.Module { return nil }
		type Requires struct{}
		type Provides struct{}
		func NewComponent(_ Requires) Provides {
			return Provides{}
		}`)
}

func TestMissingRequires(t *testing.T) {
	runLinter(t, fmt.Sprintf(
		`package component // want "%v"
		func Module() fxutil.Module { return nil }		
		type Provides struct{}
		func NewComponent(_ Requires) Provides {
			return 0
		}`, missingRequires()))

}

func TestMissingProvides(t *testing.T) {
	runLinter(t, fmt.Sprintf(
		`package component // want "%v"
		func Module() fxutil.Module { return nil }	
		type Requires struct{}
		func NewComponent(_ Requires) Provides {
			return 0
		}`, missingProvides()))
}

func TestMissingConstructor(t *testing.T) {
	runLinter(t, fmt.Sprintf(
		`package component // want "%v"
		func Module() fxutil.Module { return nil }
		type Requires struct{}
		type Provides struct{}
		func NewComponent(_ int) Provides {
			return 0
		}`, missingConstructor()))

}

func runLinter(t *testing.T, content string) {
	analyzer := &analysis.Analyzer{Run: run, Name: "compimpl", Doc: "doc"}
	// We do this to skip issues with import or other errors.
	// We only care about parsing the test file and run the analyzer.
	analyzer.RunDespiteErrors = true

	folder := filepath.Join(t.TempDir(), "testdata", "comp", "bundle", "compname", "impl")
	err := os.MkdirAll(folder, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(folder, "component.go"), []byte(content), 0644)
	require.NoError(t, err)

	analysistest.Run(t, folder, analyzer, "./...")
}
