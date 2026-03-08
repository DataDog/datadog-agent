// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

const dockerBashImage = "bash:5.2"

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
	Envs map[string]string `yaml:"envs"`
	// InterpreterEnv sets initial environment variables for the restricted
	// interpreter via the Env RunnerOption. These are passed as "KEY=value" pairs.
	InterpreterEnv map[string]string `yaml:"interpreter_env"`
	Script         string            `yaml:"script"`
	AllowedPaths   []string          `yaml:"allowed_paths"` // relative to test temp dir; "$DIR" resolves to temp dir itself
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

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(sc.Input.Script), "")
	require.NoError(t, err, "failed to parse script")

	var stdout, stderr bytes.Buffer
	opts := []interp.RunnerOption{
		interp.StdIO(nil, &stdout, &stderr),
	}
	if len(sc.Input.InterpreterEnv) > 0 {
		pairs := make([]string, 0, len(sc.Input.InterpreterEnv))
		for k, v := range sc.Input.InterpreterEnv {
			pairs = append(pairs, k+"="+v)
		}
		opts = append(opts, interp.Env(pairs...))
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

// dockerScenario associates a scenario with its test name and subdirectory
// inside the shared Docker mount.
type dockerScenario struct {
	testName string // e.g. "cmd/echo/basic"
	subdir   string // e.g. "s42"
	sc       scenario
}

// targetsLinux returns true if the scenario should run in a Linux Docker container.
func targetsLinux(sc scenario) bool {
	if len(sc.TargetOS) == 0 {
		return true
	}
	for _, goos := range sc.TargetOS {
		if goos == "linux" {
			return true
		}
	}
	return false
}

// setupTestDirIn creates a subdirectory named subdir inside parentDir and
// populates it with the scenario's setup files. The script is written to
// scriptsDir/<subdir>.sh so it doesn't pollute the working directory (which
// would break glob-based scenarios).
func setupTestDirIn(t *testing.T, parentDir, scriptsDir, subdir string, sc scenario) {
	t.Helper()
	dir := filepath.Join(parentDir, subdir)
	require.NoError(t, os.MkdirAll(dir, 0755))
	for _, f := range sc.Setup.Files {
		fullPath := filepath.Join(dir, f.Path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755), "failed to create directories for %s", f.Path)
		if f.Symlink != "" {
			require.NoError(t, os.Symlink(f.Symlink, fullPath), "failed to create symlink %s -> %s", f.Path, f.Symlink)
		} else {
			require.NoError(t, os.WriteFile(fullPath, []byte(f.Content), 0644), "failed to write file %s", f.Path)
			if f.Chmod != 0 {
				require.NoError(t, os.Chmod(fullPath, f.Chmod), "failed to chmod file %s", f.Path)
			}
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, subdir+".sh"), []byte(sc.Input.Script), 0644))
}

// buildRunnerScript generates a bash script that executes all scenarios and
// writes results (stdout, stderr, exit code) to /work/results/<subdir>.
// Scripts live in /work/scripts/<subdir>.sh, separate from the working dirs.
func buildRunnerScript(scenarios []dockerScenario) string {
	var b strings.Builder
	b.WriteString("#!/bin/bash\nmkdir -p /work/results\n")
	for _, ds := range scenarios {
		var envPrefix string
		for k, v := range ds.sc.Input.Envs {
			envPrefix += fmt.Sprintf("export %s=%s; ", k, shellQuote(v))
		}
		fmt.Fprintf(&b,
			"( cd /work/%s && %sbash /work/scripts/%s.sh ) >'/work/results/%s.stdout' 2>'/work/results/%s.stderr'; echo $? >'/work/results/%s.ec'\n",
			ds.subdir, envPrefix, ds.subdir, ds.subdir, ds.subdir, ds.subdir,
		)
	}
	return b.String()
}

// shellQuote returns a single-quoted shell string, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func TestShellScenariosAgainstBash(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found, skipping bash comparison tests")
	}
	// Pull the image once before starting the container.
	pull := exec.Command("docker", "pull", "-q", dockerBashImage)
	if out, err := pull.CombinedOutput(); err != nil {
		t.Skipf("failed to pull %s docker image: %v\n%s", dockerBashImage, err, out)
	}

	// Create a shared temp directory that will be bind-mounted into the container.
	sharedDir := t.TempDir()

	// --- Phase 1: collect all eligible scenarios and write their files ---
	scenariosDir := filepath.Join("scenarios")
	groups := discoverScenarioFiles(t, scenariosDir)
	require.NotEmpty(t, groups, "no scenario files found in %s", scenariosDir)

	scriptsDir := filepath.Join(sharedDir, "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0755))

	var allScenarios []dockerScenario
	seq := 0
	for group, paths := range groups {
		for _, path := range paths {
			sc := loadScenario(t, path)
			if sc.TestAgainstLocalShell != nil && !*sc.TestAgainstLocalShell {
				continue
			}
			if !targetsLinux(sc) {
				continue
			}
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			subdir := fmt.Sprintf("s%d", seq)
			seq++
			setupTestDirIn(t, sharedDir, scriptsDir, subdir, sc)
			allScenarios = append(allScenarios, dockerScenario{
				testName: group + "/" + name,
				subdir:   subdir,
				sc:       sc,
			})
		}
	}
	require.NotEmpty(t, allScenarios, "no eligible scenarios found")

	// --- Phase 2: run ALL scenarios in a single docker invocation ---
	runnerScript := buildRunnerScript(allScenarios)
	runnerPath := filepath.Join(sharedDir, "runner.sh")
	require.NoError(t, os.WriteFile(runnerPath, []byte(runnerScript), 0755))

	cmd := exec.Command("docker", "run", "--rm",
		"-v", sharedDir+":/work",
		dockerBashImage, "bash", "/work/runner.sh",
	)
	var dockerStderr bytes.Buffer
	cmd.Stderr = &dockerStderr
	require.NoError(t, cmd.Run(), "runner script failed: %s", dockerStderr.String())

	// --- Phase 3: read results and assert per-scenario expectations ---
	resultsDir := filepath.Join(sharedDir, "results")
	for _, ds := range allScenarios {
		t.Run(ds.testName, func(t *testing.T) {
			stdout, err := os.ReadFile(filepath.Join(resultsDir, ds.subdir+".stdout"))
			require.NoError(t, err, "missing stdout for %s", ds.testName)
			stderr, err := os.ReadFile(filepath.Join(resultsDir, ds.subdir+".stderr"))
			require.NoError(t, err, "missing stderr for %s", ds.testName)
			ecBytes, err := os.ReadFile(filepath.Join(resultsDir, ds.subdir+".ec"))
			require.NoError(t, err, "missing exit code for %s", ds.testName)
			exitCode, err := strconv.Atoi(strings.TrimSpace(string(ecBytes)))
			require.NoError(t, err, "invalid exit code for %s: %q", ds.testName, string(ecBytes))

			assertExpectations(t, ds.sc, string(stdout), string(stderr), exitCode)
		})
	}
}

func TestShellScenarios(t *testing.T) {
	scenariosDir := filepath.Join("scenarios")
	groups := discoverScenarioFiles(t, scenariosDir)
	require.NotEmpty(t, groups, "no scenario files found in %s", scenariosDir)

	for group, paths := range groups {
		t.Run(group, func(t *testing.T) {
			t.Parallel()
			for _, path := range paths {
				sc := loadScenario(t, path)
				name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					runScenario(t, sc)
				})
			}
		})
	}
}
