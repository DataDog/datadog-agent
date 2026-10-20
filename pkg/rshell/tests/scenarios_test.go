// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

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
	"time"

	"slices"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

const (
	dockerBashImage  = "debian:bookworm-slim"
	dockerJQVersion  = "1.8.1"
	dockerJQAMD64SHA = "020468de7539ce70ef1bceaf7cde2e8c4f2ca6c3afb84642aabc5c97d9fc2a0d"
	dockerJQARM64SHA = "6bc62f25981328edd3cfcfe6fe51b073f2d7e7710d7ef7fcdac28d4e384fc3d4"
)

// scenario represents a single test scenario.
type scenario struct {
	Description           string `yaml:"description"`
	SkipAssertAgainstBash bool   `yaml:"skip_assert_against_bash"` // true = skip bash comparison
	// SkipWindows skips the entire scenario on Windows, for commands that are
	// intentionally unsupported on that platform (e.g. vmstat) rather than
	// merely producing different output.
	SkipWindows bool `yaml:"skip_windows"`
	// Containerized enables container symlink resolution by setting
	// HostPrefix to the test directory's host/ subdirectory.
	Containerized bool     `yaml:"containerized"`
	Setup         setup    `yaml:"setup"`
	Input         input    `yaml:"input"`
	Expect        expected `yaml:"expect"`
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
	Symlink string      `yaml:"symlink"`  // if set, create a symlink pointing to this target (relative to test dir)
	ModTime string      `yaml:"mod_time"` // if set, override the file's modification time (RFC 3339 format)
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
	// AllowedCommands lists the command names (builtin or external) that the
	// interpreter is permitted to execute. If nil and AllowAllCommands is not
	// explicitly set to true, the test defaults to allowing all commands for
	// backward compatibility.
	AllowedCommands []string `yaml:"allowed_commands"`
	// AllowAllCommands permits any command to be executed, bypassing the
	// AllowedCommands restriction. When explicitly set to false in a
	// scenario, no commands are allowed. When omitted, the test harness
	// defaults to allowing all commands for backward compatibility.
	AllowAllCommands *bool `yaml:"allow_all_commands"`
	// Mode sets the runner execution mode. Accepted values: "read-only" (default), "remediation".
	// "remediation" enables file-target output redirections (>, >>, 2>, &>, &>>) within read-write AllowedPaths.
	Mode string `yaml:"mode"`
	// AllowedSystemServices configures exact systemd unit/action grants for
	// systemd-aware builtins. When omitted, no grant is configured and every
	// systemd operation is denied, which is the default for every scenario.
	// Scenarios must not rely on a live systemd manager: only grant shapes
	// whose outcome is decided by policy before the manager is contacted.
	AllowedSystemServices []systemServiceGrant `yaml:"allowed_system_services"`
}

// systemServiceGrant is the YAML form of interp.SystemServiceControlGrant.
type systemServiceGrant struct {
	Service string   `yaml:"service"`
	Actions []string `yaml:"actions"`
}

// expected holds the expected output for a scenario.
type expected struct {
	Stdout                string   `yaml:"stdout"`
	StdoutUnordered       string   `yaml:"stdout_unordered"`
	StdoutWindows         *string  `yaml:"stdout_windows"`
	StdoutContains        []string `yaml:"stdout_contains"`
	StdoutContainsWindows []string `yaml:"stdout_contains_windows"`
	Stderr                string   `yaml:"stderr"`
	StderrWindows         *string  `yaml:"stderr_windows"`
	StderrContains        []string `yaml:"stderr_contains"`
	StderrContainsWindows []string `yaml:"stderr_contains_windows"`
	ExitCode              int      `yaml:"exit_code"`
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

func splitScenarioPathMode(path string) (base string, suffix string) {
	for _, candidate := range []string{":ro", ":rw"} {
		if strings.HasSuffix(path, candidate) && len(path) > len(candidate) {
			return path[:len(path)-len(candidate)], candidate
		}
	}
	return path, ""
}

func resolveScenarioAllowedPath(dir string, configuredPath string) (string, bool) {
	path, suffix := splitScenarioPathMode(configuredPath)
	var resolved string
	switch {
	case path == "$DIR":
		resolved = dir
	case filepath.IsAbs(path) || strings.HasPrefix(path, "/"):
		// Absolute paths (e.g. /proc/net) are used as-is to allow access
		// to kernel virtual filesystems that live outside the test temp dir.
		// Also handle Unix-style paths starting with "/" on Windows, where
		// filepath.IsAbs only recognises drive-letter paths like C:\...
		// Skip if the path does not exist on this OS (e.g. /proc/net on macOS/Windows).
		if _, err := os.Stat(path); err != nil {
			return "", false
		}
		resolved = path
	default:
		resolved = filepath.Join(dir, path)
	}
	if suffix != "" {
		return resolved + suffix, true
	}
	return resolved, true
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
		if f.ModTime != "" {
			mt, err := time.Parse(time.RFC3339, f.ModTime)
			require.NoError(t, err, "failed to parse mod_time for %s", f.Path)
			require.NoError(t, os.Chtimes(fullPath, mt, mt), "failed to set mod_time for %s", f.Path)
		}
	}
	return dir
}

// runScenario executes a single test scenario against the shell interpreter
// and asserts the expected output.
func runScenario(t *testing.T, sc scenario) {
	t.Helper()

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
		var resolved []string
		for _, p := range sc.Input.AllowedPaths {
			path, ok := resolveScenarioAllowedPath(dir, p)
			if ok {
				resolved = append(resolved, path)
			}
		}
		// Always apply AllowedPaths when the scenario specifies it, even
		// if resolved is empty (e.g. all paths are /proc/net on macOS).
		// An empty list enforces a closed sandbox rather than leaving the
		// runner unrestricted.
		opts = append(opts, interp.AllowedPaths(resolved))
	}
	if sc.Input.AllowAllCommands != nil && *sc.Input.AllowAllCommands {
		opts = append(opts, interpoption.AllowAllCommands().(interp.RunnerOption))
	} else if len(sc.Input.AllowedCommands) > 0 {
		opts = append(opts, interp.AllowedCommands(sc.Input.AllowedCommands))
	} else if sc.Input.AllowAllCommands == nil {
		// Default: allow all commands for backward compatibility with
		// existing scenarios that predate the allowedCommands feature.
		opts = append(opts, interpoption.AllowAllCommands().(interp.RunnerOption))
	}
	// When allow_all_commands is explicitly false and allowed_commands is
	// empty, no AllowedCommands/AllowAllCommands option is added, so the
	// interpreter defaults to blocking all commands.
	if sc.Containerized {
		opts = append(opts, interp.HostPrefix(filepath.Join(dir, "host")))
	}
	if sc.Input.Mode == string(interp.ModeRemediation) {
		opts = append(opts, interp.WithMode(interp.ModeRemediation))
	}
	if sc.Input.AllowedSystemServices != nil {
		grants := make([]interp.SystemServiceControlGrant, 0, len(sc.Input.AllowedSystemServices))
		for _, grant := range sc.Input.AllowedSystemServices {
			actions := make([]interp.SystemServiceAction, 0, len(grant.Actions))
			for _, action := range grant.Actions {
				actions = append(actions, interp.SystemServiceAction(action))
			}
			grants = append(grants, interp.SystemServiceControlGrant{Service: grant.Service, Actions: actions})
		}
		opts = append(opts, interp.AllowedSystemServices(grants))
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
// On Windows, stdout_windows/stderr_windows are used when set, falling back to stdout/stderr.
func assertExpectations(t *testing.T, sc scenario, stdout, stderr string, exitCode int) {
	t.Helper()

	expectedStdout := sc.Expect.Stdout
	expectedStderr := sc.Expect.Stderr
	if runtime.GOOS == "windows" {
		if sc.Expect.StdoutWindows != nil {
			expectedStdout = *sc.Expect.StdoutWindows
		}
		if sc.Expect.StderrWindows != nil {
			expectedStderr = *sc.Expect.StderrWindows
		}
	}

	assert.Equal(t, sc.Expect.ExitCode, exitCode, "exit code mismatch")

	stdoutContains := sc.Expect.StdoutContains
	if runtime.GOOS == "windows" && len(sc.Expect.StdoutContainsWindows) > 0 {
		stdoutContains = sc.Expect.StdoutContainsWindows
	}
	stderrContains := sc.Expect.StderrContains
	if runtime.GOOS == "windows" && len(sc.Expect.StderrContainsWindows) > 0 {
		stderrContains = sc.Expect.StderrContainsWindows
	}

	if len(stdoutContains) > 0 {
		for _, substr := range stdoutContains {
			assert.Contains(t, stdout, substr, "stdout should contain %q", substr)
		}
	} else if sc.Expect.StdoutUnordered != "" {
		wantLines := strings.Split(sc.Expect.StdoutUnordered, "\n")
		gotLines := strings.Split(stdout, "\n")
		slices.Sort(wantLines)
		slices.Sort(gotLines)
		assert.Equal(t, wantLines, gotLines, "stdout mismatch (unordered)")
	} else {
		assert.Equal(t, expectedStdout, stdout, "stdout mismatch")
	}
	if len(stderrContains) > 0 {
		for _, substr := range stderrContains {
			assert.Contains(t, stderr, substr, "stderr should contain %q", substr)
		}
	} else {
		assert.Equal(t, expectedStderr, stderr, "stderr mismatch")
	}
}

// dockerScenario associates a scenario with its test name and subdirectory
// inside the shared Docker mount.
type dockerScenario struct {
	testName string // e.g. "cmd/echo/basic"
	subdir   string // e.g. "s42"
	sc       scenario
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
		if f.ModTime != "" {
			mt, err := time.Parse(time.RFC3339, f.ModTime)
			require.NoError(t, err, "failed to parse mod_time for %s", f.Path)
			require.NoError(t, os.Chtimes(fullPath, mt, mt), "failed to set mod_time for %s", f.Path)
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(scriptsDir, subdir+".sh"), []byte(sc.Input.Script), 0644))
}

// buildRunnerScript generates a bash script that executes all scenarios and
// writes results (stdout, stderr, exit code) to /work/results/<subdir>.
// Scripts live in /work/scripts/<subdir>.sh, separate from the working dirs.
func buildRunnerScript(scenarios []dockerScenario) string {
	needsBinutils := false
	needsJQ := false
	for _, ds := range scenarios {
		if strings.Contains(ds.testName, "cmd/strings/") {
			needsBinutils = true
		}
		if strings.Contains(ds.testName, "cmd/jq/") {
			needsJQ = true
		}
	}

	var b strings.Builder
	b.WriteString("#!/bin/bash\n")
	if needsBinutils || needsJQ {
		var packages []string
		if needsBinutils {
			packages = append(packages, "binutils")
		}
		if needsJQ {
			packages = append(packages, "ca-certificates", "curl")
		}
		b.WriteString("apt-get update -qq && apt-get install -y -qq ")
		b.WriteString(strings.Join(packages, " "))
		b.WriteString(" >/dev/null 2>&1 || exit 1\n")
	}
	if needsJQ {
		b.WriteString("case \"$(uname -m)\" in\n")
		fmt.Fprintf(&b, "  x86_64) jq_asset=jq-linux-amd64; jq_sha=%s ;;\n", dockerJQAMD64SHA)
		fmt.Fprintf(&b, "  aarch64|arm64) jq_asset=jq-linux-arm64; jq_sha=%s ;;\n", dockerJQARM64SHA)
		b.WriteString("  *) echo 'unsupported architecture for jq reference binary' >&2; exit 1 ;;\n")
		b.WriteString("esac\n")
		fmt.Fprintf(&b, "curl -fsSL --retry 3 https://github.com/jqlang/jq/releases/download/jq-%s/$jq_asset -o /tmp/jq-reference || exit 1\n", dockerJQVersion)
		b.WriteString("echo \"$jq_sha  /tmp/jq-reference\" | sha256sum -c - >/dev/null || exit 1\n")
		b.WriteString("install -m 0755 /tmp/jq-reference /usr/local/bin/jq || exit 1\n")
		fmt.Fprintf(&b, "test \"$(jq --version)\" = \"jq-%s\" || exit 1\n", dockerJQVersion)
	}
	b.WriteString("mkdir -p /work/results\n")
	b.WriteString("cleanup() { chmod -R 777 /work/results 2>/dev/null; }\ntrap cleanup EXIT\n")
	for _, ds := range scenarios {
		var envParts []string
		for k, v := range ds.sc.Input.Envs {
			envParts = append(envParts, fmt.Sprintf("export %s=%s;", k, shellQuote(v)))
		}
		envPrefix := ""
		if len(envParts) > 0 {
			envPrefix = strings.Join(envParts, " ") + " "
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
	if os.Getenv("RSHELL_BASH_TEST") == "" {
		t.Skip("skipping bash comparison tests (set RSHELL_BASH_TEST=1 to enable)")
	}
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
			// AWK uses checked-in expectations; Debian's mawk is not an oracle for the restricted builtin.
			if sc.SkipAssertAgainstBash || strings.Contains(filepath.ToSlash(path), "scenarios/cmd/awk/") {
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
					if sc.Containerized && runtime.GOOS == "windows" {
						t.Skip("containerized tests are not supported on Windows")
					}
					if sc.SkipWindows && runtime.GOOS == "windows" {
						t.Skip("scenario is not supported on Windows")
					}
					t.Parallel()
					runScenario(t, sc)
				})
			}
		})
	}
}
