// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tests

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	Description           string   `yaml:"description"`
	TargetOS              []string `yaml:"target_os"`                // if set, only run on these OS (linux, darwin, windows); empty means all
	TestAgainstLocalShell *bool    `yaml:"test_against_local_shell"` // nil = true (default); false = skip bash comparison
	Setup                 setup    `yaml:"setup"`
	Input                 input    `yaml:"input"`
	Expect                expected `yaml:"expect"`
}

// setup holds optional pre-test configuration such as files to create.
type setup struct {
	Files []setupFile `yaml:"files"`
}

// setupFile describes a file to create before executing the scenario.
// When Symlink is set, a symbolic link is created instead of a regular file.
type setupFile struct {
	Path    string      `yaml:"path"`
	Content string      `yaml:"content"`
	Chmod   os.FileMode `yaml:"chmod"`
	Symlink string      `yaml:"symlink"` // if set, create a symlink pointing to this target (relative to test dir)
}

// input holds the shell script to execute.
type input struct {
	// Envs sets OS-level environment variables for the bash comparison test
	// only. These are intentionally NOT passed to the restricted interpreter,
	// which starts with an empty environment for security (no host env inheritance).
	Envs         map[string]string `yaml:"envs"`
	Script       string            `yaml:"script"`
	AllowedPaths []string          `yaml:"allowed_paths"` // relative to test temp dir; "$DIR" resolves to temp dir itself
}

// expected holds the expected output for a scenario.
type expected struct {
	Stdout         string   `yaml:"stdout"`
	StdoutContains []string `yaml:"stdout_contains"`
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
		if f.Symlink != "" {
			// Create a symbolic link. The target is used as-is (relative to the link's location).
			require.NoError(t, os.Symlink(f.Symlink, fullPath), "failed to create symlink %s -> %s", f.Path, f.Symlink)
		} else {
			require.NoError(t, os.WriteFile(fullPath, []byte(f.Content), 0644), "failed to write file %s", f.Path)
			if f.Chmod != 0 {
				require.NoError(t, os.Chmod(fullPath, f.Chmod), "failed to chmod file %s", f.Path)
			}
		}
	}
	return dir
}

// runScenario executes a single test scenario against the shell interpreter
// and asserts the expected output.
func runScenario(t *testing.T, sc scenario) {
	t.Helper()

	if len(sc.TargetOS) > 0 {
		matched := false
		for _, goos := range sc.TargetOS {
			if goos == runtime.GOOS {
				matched = true
				break
			}
		}
		if !matched {
			t.Skipf("skipping: scenario targets %v, current GOOS is %s", sc.TargetOS, runtime.GOOS)
		}
	}

	dir := setupTestDir(t, sc)

	// Set OS-level env vars so the bash comparison path (runScenarioAgainstBash)
	// picks them up via os.Environ(). The restricted interpreter intentionally
	// does NOT inherit these — it starts with an empty environment for security.
	for k, v := range sc.Input.Envs {
		t.Setenv(k, v)
	}

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(sc.Input.Script), "")
	require.NoError(t, err, "failed to parse script")

	var stdout, stderr bytes.Buffer
	opts := []interp.RunnerOption{
		interp.StdIO(nil, &stdout, &stderr),
	}
	if sc.Input.AllowedPaths != nil {
		resolved := make([]string, len(sc.Input.AllowedPaths))
		for i, p := range sc.Input.AllowedPaths {
			if p == "$DIR" {
				resolved[i] = dir
			} else {
				resolved[i] = filepath.Join(dir, p)
			}
		}
		opts = append(opts, interp.AllowedPaths(resolved))
	}
	runner, err := interp.New(opts...)
	require.NoError(t, err, "failed to create runner")
	defer runner.Close()

	runner.Dir = dir

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

	assertExpectations(t, sc, stdout.String(), stderr.String(), exitCode)
}

// assertExpectations checks stdout, stderr, and exit code against the scenario expectations.
func assertExpectations(t *testing.T, sc scenario, stdout, stderr string, exitCode int) {
	t.Helper()

	assert.Equal(t, sc.Expect.ExitCode, exitCode, "exit code mismatch")
	if len(sc.Expect.StdoutContains) > 0 {
		for _, substr := range sc.Expect.StdoutContains {
			assert.Contains(t, stdout, substr, "stdout should contain %q", substr)
		}
	} else {
		assert.Equal(t, sc.Expect.Stdout, stdout, "stdout mismatch")
	}
	if len(sc.Expect.StderrContains) > 0 {
		for _, substr := range sc.Expect.StderrContains {
			assert.Contains(t, stderr, substr, "stderr should contain %q", substr)
		}
	} else {
		assert.Equal(t, sc.Expect.Stderr, stderr, "stderr mismatch")
	}
}

// runScenarioAgainstBash executes a scenario against real /bin/bash and asserts expectations.
func runScenarioAgainstBash(t *testing.T, sc scenario) {
	t.Helper()

	if len(sc.TargetOS) > 0 {
		matched := false
		for _, goos := range sc.TargetOS {
			if goos == runtime.GOOS {
				matched = true
				break
			}
		}
		if !matched {
			t.Skipf("skipping: scenario targets %v, current GOOS is %s", sc.TargetOS, runtime.GOOS)
		}
	}

	dir := setupTestDir(t, sc)

	env := os.Environ()
	for k, v := range sc.Input.Envs {
		env = append(env, k+"="+v)
	}

	cmd := exec.Command("/bin/bash", "-c", sc.Input.Script)
	cmd.Dir = dir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running bash: %v", err)
		}
	}

	assertExpectations(t, sc, stdout.String(), stderr.String(), exitCode)
}

func TestShellScenariosAgainstBash(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("bash comparison tests only run on linux and darwin")
	}
	if _, err := exec.LookPath("/bin/bash"); err != nil {
		t.Skip("/bin/bash not found, skipping bash comparison tests")
	}

	scenariosDir := filepath.Join("scenarios")
	groups := discoverScenarioFiles(t, scenariosDir)
	require.NotEmpty(t, groups, "no scenario files found in %s", scenariosDir)

	for group, paths := range groups {
		t.Run(group, func(t *testing.T) {
			for _, path := range paths {
				sc := loadScenario(t, path)
				if sc.TestAgainstLocalShell != nil && !*sc.TestAgainstLocalShell {
					continue
				}
				name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				t.Run(name, func(t *testing.T) {
					runScenarioAgainstBash(t, sc)
				})
			}
		})
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
