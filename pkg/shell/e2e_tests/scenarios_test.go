// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e_tests

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// scenario represents a single test scenario.
type scenario struct {
	Description string   `yaml:"description"`
	Setup       setup    `yaml:"setup"`
	Input       input    `yaml:"input"`
	Expected    expected `yaml:"expected"`
}

// setup holds optional pre-test configuration such as files to create.
type setup struct {
	Files []setupFile `yaml:"files"`
}

// setupFile describes a file to create before executing the scenario.
type setupFile struct {
	Path    string      `yaml:"path"`
	Content string      `yaml:"content"`
	Chmod   os.FileMode `yaml:"chmod"`
}

// input holds the shell script to execute.
type input struct {
	Envs   map[string]string `yaml:"envs"`
	Script string            `yaml:"script"`
}

// expected holds the expected output for a scenario.
type expected struct {
	Stdout         string   `yaml:"stdout"`
	Stderr         string   `yaml:"stderr"`
	StderrContains []string `yaml:"stderr_contains"`
	ExitCode       int      `yaml:"exit_code"`
}

// discoverScenarioFiles walks the scenarios directory and returns all YAML files
// grouped by their relative directory path.
func discoverScenarioFiles(t *testing.T, scenariosDir string) map[string][]string {
	t.Helper()
	files := make(map[string][]string)
	err := filepath.Walk(scenariosDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		rel, err := filepath.Rel(scenariosDir, path)
		if err != nil {
			return err
		}
		group := filepath.Dir(rel)
		files[group] = append(files[group], path)
		return nil
	})
	require.NoError(t, err, "failed to walk scenarios directory")
	return files
}

// loadScenario parses a YAML file into a single scenario.
func loadScenario(t *testing.T, path string) scenario {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read scenario file %s", path)

	var sc scenario
	err = yaml.Unmarshal(data, &sc)
	require.NoError(t, err, "failed to parse scenario file %s", path)
	return sc
}

// setupTestDir creates a temporary directory and populates it with any files
// defined in the scenario's setup section. It returns the path to the temp dir.
func setupTestDir(t *testing.T, sc scenario) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range sc.Setup.Files {
		fullPath := filepath.Join(dir, f.Path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755), "failed to create directories for %s", f.Path)
		require.NoError(t, os.WriteFile(fullPath, []byte(f.Content), 0644), "failed to write file %s", f.Path)
		if f.Chmod != 0 {
			require.NoError(t, os.Chmod(fullPath, f.Chmod), "failed to chmod file %s", f.Path)
		}
	}
	return dir
}

// runScenario executes a single test scenario against the shell interpreter
// and asserts the expected output.
func runScenario(t *testing.T, sc scenario) {
	t.Helper()

	dir := setupTestDir(t, sc)

	for k, v := range sc.Input.Envs {
		t.Setenv(k, v)
	}

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(sc.Input.Script), "")
	require.NoError(t, err, "failed to parse script")

	var stdout, stderr bytes.Buffer
	runner, err := interp.New(
		interp.Dir(dir),
		interp.StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err, "failed to create runner")

	err = runner.Run(context.Background(), prog)

	// Extract exit code from error.
	exitCode := 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	assert.Equal(t, sc.Expected.ExitCode, exitCode, "exit code mismatch")
	assert.Equal(t, sc.Expected.Stdout, stdout.String(), "stdout mismatch")
	if len(sc.Expected.StderrContains) > 0 {
		for _, substr := range sc.Expected.StderrContains {
			assert.Contains(t, stderr.String(), substr, "stderr should contain %q", substr)
		}
	} else {
		assert.Equal(t, sc.Expected.Stderr, stderr.String(), "stderr mismatch")
	}
}

func TestShellScenarios(t *testing.T) {
	scenariosDir := filepath.Join("scenarios")
	groups := discoverScenarioFiles(t, scenariosDir)
	require.NotEmpty(t, groups, "no scenario files found in %s", scenariosDir)

	for group, paths := range groups {
		t.Run(group, func(t *testing.T) {
			for _, path := range paths {
				sc := loadScenario(t, path)
				name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				t.Run(name, func(t *testing.T) {
					runScenario(t, sc)
				})
			}
		})
	}
}
