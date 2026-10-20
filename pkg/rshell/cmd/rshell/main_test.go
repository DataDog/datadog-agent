// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/version"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runCLI(t *testing.T, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	return runCLIContext(t, context.Background(), args...)
}

func runCLIContext(t *testing.T, ctx context.Context, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := run(ctx, args, strings.NewReader(""), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestEcho(t *testing.T) {
	code, stdout, _ := runCLI(t, "--allow-all-commands", "-c", `echo hello world`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

func TestShortFlag(t *testing.T) {
	code, stdout, _ := runCLI(t, "--allow-all-commands", "-c", `echo short`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "short\n", stdout)
}

func runCLIWithStdin(t *testing.T, stdin string, args ...string) (exitCode int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := run(context.Background(), args, strings.NewReader(stdin), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestStdin(t *testing.T) {
	code, stdout, _ := runCLIWithStdin(t, "echo from-stdin\n", "--allow-all-commands")
	assert.Equal(t, 0, code)
	assert.Equal(t, "from-stdin\n", stdout)
}

func TestEmptyStdin(t *testing.T) {
	code, stdout, stderr := runCLIWithStdin(t, "", "--allow-all-commands")
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestEmptyCommand(t *testing.T) {
	code, stdout, stderr := runCLI(t, "-c", "")
	assert.Equal(t, 0, code, "empty command should exit 0 (matching bash -c '')")
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestExitCode(t *testing.T) {
	code, _, _ := runCLI(t, "--allow-all-commands", "-c", `exit 42`)
	assert.Equal(t, 42, code)
}

func TestParseError(t *testing.T) {
	code, _, stderr := runCLI(t, "-c", `echo "unterminated`)
	assert.Equal(t, 2, code, "parse errors should return exit code 2 (matching bash)")
	assert.Contains(t, stderr, "without closing quote")
}

func TestParseErrorSyntax(t *testing.T) {
	code, _, stderr := runCLI(t, "-c", `if; then`)
	assert.Equal(t, 2, code, "syntax errors should return exit code 2 (matching bash)")
	assert.Contains(t, stderr, "must be followed by")
}

func TestParseErrorUnclosed(t *testing.T) {
	code, _, stderr := runCLI(t, "-c", "if true; then\n  echo hello")
	assert.Equal(t, 2, code, "unclosed blocks should return exit code 2 (matching bash)")
	assert.Contains(t, stderr, "must end with")
}

func setupTestFile(t *testing.T) (dir, filePath string) {
	t.Helper()
	dir = t.TempDir()
	filePath = filepath.Join(dir, "testfile.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello from testfile\n"), 0o644))
	if runtime.GOOS == "windows" {
		filePath = filepath.ToSlash(filePath)
		dir = filepath.ToSlash(dir)
	}
	return dir, filePath
}

func TestFileAccessDeniedByDefault(t *testing.T) {
	_, filePath := setupTestFile(t)
	code, _, stderr := runCLI(t, "--allow-all-commands", "-c", `cat `+filePath)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "permission denied")
}

func TestAllowedPathGrantsAccess(t *testing.T) {
	dir, filePath := setupTestFile(t)
	code, stdout, _ := runCLI(t, "--allow-all-commands", "-c", `cat `+filePath, "-p", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "hello from testfile")
}

func TestAllowedPathCommaSeparated(t *testing.T) {
	dir, filePath := setupTestFile(t)
	extraDir := t.TempDir()
	if runtime.GOOS == "windows" {
		extraDir = filepath.ToSlash(extraDir)
	}
	code, stdout, _ := runCLI(t, "--allow-all-commands", "-c", `cat `+filePath, "--allowed-paths", dir+","+extraDir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "hello from testfile")
}

// TestDefaultDirIsFirstAllowedPath verifies that when AllowedPaths is set, the
// shell starts in the first allowed path so relative file access works without
// the caller having to chdir first.
func TestDefaultDirIsFirstAllowedPath(t *testing.T) {
	dir, _ := setupTestFile(t)
	// "testfile.txt" is a path relative to the working directory. If the
	// shell defaults to the first allowed path, this resolves to dir/testfile.txt.
	code, stdout, stderr := runCLI(t, "--allow-all-commands", "-c", `cat testfile.txt`, "-p", dir)
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "hello from testfile")
}

// TestDefaultDirPicksFirstOfMany verifies that the shell starts in the *first*
// allowed path when several are configured, not a later one.
func TestDefaultDirPicksFirstOfMany(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(first, "marker.txt"), []byte("first"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(second, "marker.txt"), []byte("second"), 0o644))
	if runtime.GOOS == "windows" {
		first = filepath.ToSlash(first)
		second = filepath.ToSlash(second)
	}
	code, stdout, stderr := runCLI(t, "--allow-all-commands", "-c", `cat marker.txt`, "-p", first+","+second)
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Equal(t, "first", stdout, "should read marker.txt from the first allowed path")
}

func TestMultipleStatements(t *testing.T) {
	code, stdout, _ := runCLI(t, "--allow-all-commands", "-c", "echo first\necho second")
	assert.Equal(t, 0, code)
	assert.Equal(t, "first\nsecond\n", stdout)
}

func TestVariableExpansion(t *testing.T) {
	code, stdout, _ := runCLI(t, "--allow-all-commands", "-c", `FOO=bar; echo $FOO`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "bar\n", stdout)
}

func TestHelp(t *testing.T) {
	code, stdout, _ := runCLI(t, "--help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "--allowed-paths")
	assert.Contains(t, stdout, "PATH[:ro|:rw]")
	assert.Contains(t, stdout, "entries without a suffix are read-only")
	assert.Contains(t, stdout, "--allowed-commands")
	assert.Contains(t, stdout, "--allowed-services")
	assert.Contains(t, stdout, "UNIT:ACTION[+ACTION...]")
	assert.Contains(t, stdout, "--allow-all-commands")
	assert.Contains(t, stdout, "file-target output redirections within :rw AllowedPaths roots")
	assert.Contains(t, stdout, "remediation-only builtins, including the restricted systemctl builtin")
	assert.Contains(t, stdout, "--timeout")
	assert.Contains(t, stdout, "--systemd-journal-dirs")
	assert.Contains(t, stdout, "--systemd-machine-id-path")
	assert.Contains(t, stdout, "--systemd-journal-socket")
	assert.Contains(t, stdout, "--systemd-manager-socket")
	assert.NotContains(t, stdout, "--command", "-c/--command should be hidden from help")
}

// TestVersion verifies that --version exits 0 and prints the version.
// In tests rshell is the main module, so debug.ReadBuildInfo returns "(devel)"
// and the version falls back to "dev". When imported as a library (e.g. by the
// Datadog Agent) it reports the real version from go.mod — see
// TestBuildVersionAsDependency in internal/version/.
func TestVersion(t *testing.T) {
	code, stdout, _ := runCLI(t, "--version")
	t.Logf("stdout: %q", stdout)
	assert.Equal(t, 0, code)
	assert.Equal(t, "rshell version "+version.Version+"\n", stdout)
}

func TestFileArg(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "test.sh")
	require.NoError(t, os.WriteFile(script, []byte("echo from-file\n"), 0o644))

	code, stdout, _ := runCLI(t, "--allow-all-commands", script)
	assert.Equal(t, 0, code)
	assert.Equal(t, "from-file\n", stdout)
}

func TestMultipleFileArgs(t *testing.T) {
	dir := t.TempDir()
	script1 := filepath.Join(dir, "a.sh")
	script2 := filepath.Join(dir, "b.sh")
	require.NoError(t, os.WriteFile(script1, []byte("echo first\n"), 0o644))
	require.NoError(t, os.WriteFile(script2, []byte("echo second\n"), 0o644))

	code, stdout, _ := runCLI(t, "--allow-all-commands", script1, script2)
	assert.Equal(t, 0, code)
	assert.Equal(t, "first\nsecond\n", stdout)
}

func TestCommandAndFileArgsMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "test.sh")
	require.NoError(t, os.WriteFile(script, []byte("echo hi\n"), 0o644))

	code, _, stderr := runCLI(t, "-c", "echo hi", script)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "cannot use -c with file arguments")
}

func TestFileNotFound(t *testing.T) {
	code, _, stderr := runCLI(t, "/nonexistent/path/script.sh")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "reading /nonexistent/path/script.sh")
}

func TestFileArgWithAllowedPath(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	dataFile := filepath.Join(dataDir, "data.txt")
	require.NoError(t, os.WriteFile(dataFile, []byte("secret data\n"), 0o644))

	if runtime.GOOS == "windows" {
		dataFile = filepath.ToSlash(dataFile)
		dataDir = filepath.ToSlash(dataDir)
	}

	script := filepath.Join(dir, "test.sh")
	require.NoError(t, os.WriteFile(script, []byte("cat "+dataFile+"\n"), 0o644))

	code, stdout, _ := runCLI(t, "--allow-all-commands", "-p", dataDir, script)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "secret data")
}

func TestDefaultNoCommandsAllowed(t *testing.T) {
	code, _, stderr := runCLI(t, "-c", `echo hello`)
	assert.Equal(t, 127, code)
	assert.Contains(t, stderr, "command not allowed")
}

func TestAllowedCommandsFlag(t *testing.T) {
	code, stdout, _ := runCLI(t, "--allowed-commands", "rshell:echo", "-c", `echo hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestAllowedCommandsBlocksOther(t *testing.T) {
	code, _, stderr := runCLI(t, "--allowed-commands", "rshell:echo", "-c", `cat /dev/null`)
	assert.Equal(t, 127, code)
	assert.Contains(t, stderr, "command not allowed")
}

func TestAllowedCommandsMissingNamespace(t *testing.T) {
	code, _, stderr := runCLI(t, "--allowed-commands", "echo", "-c", `echo hello`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing namespace prefix")
}

func TestAllowedCommandsUnknownNamespace(t *testing.T) {
	code, _, stderr := runCLI(t, "--allowed-commands", "host:echo", "-c", `echo hello`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unknown namespace")
}

func TestAllowedServicesFlag(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"--allow-all-commands",
		"--allowed-services", "mysql.service:restart+reload+read,nginx.service:read",
		"--mode", "remediation",
		"-c", `echo hello`,
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
	assert.Empty(t, stderr)
}

func TestAllowedServicesFlagExpandsWildcard(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"--allow-all-commands",
		"--allowed-services", "mysql.service:*",
		"-c", `help`,
	)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "  mysql.service:read+clean+start+stop+reload+restart+enable+disable\n")
	assert.Empty(t, stderr)
}

func TestAllowedServicesFlagIgnoresGrantsWithoutActions(t *testing.T) {
	for _, grant := range []string{"mysql.service", "mysql.service:"} {
		t.Run(grant, func(t *testing.T) {
			code, stdout, stderr := runCLI(t,
				"--allow-all-commands",
				"--allowed-services", grant,
				"-c", `echo hello`,
			)
			assert.Equal(t, 0, code)
			assert.Equal(t, "hello\n", stdout)
			assert.Empty(t, stderr)
		})
	}
}

func TestAllowedServicesFlagWarnsAndSkipsUnknownAction(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"--allow-all-commands",
		"--allowed-services", "mysql.service:freeze",
		"-c", `echo hello`,
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
	assert.Contains(t, stderr, `skipping unsupported action "freeze"`)
}

func TestAllowedServicesFlagWarnsAndSkipsInvalidService(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"--allow-all-commands",
		"--allowed-services", "mysql*.service:read",
		"-c", `echo hello`,
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
	assert.Contains(t, stderr, "AllowedSystemServices: skipping")
	assert.Contains(t, stderr, "glob pattern")
}

func TestParseAllowedServicesParsesServiceActions(t *testing.T) {
	grants := parseAllowedServices("systemd-journald.service:read+clean")
	require.Len(t, grants, 1)
	assert.Equal(t, "systemd-journald.service", grants[0].Service)
	assert.Equal(t, []interp.SystemServiceAction{interp.SystemServiceRead, interp.SystemServiceClean}, grants[0].Actions)
}

func TestParseAllowedServicesParsesWildcard(t *testing.T) {
	grants := parseAllowedServices("mysql.service:*")
	require.Len(t, grants, 1)
	assert.Equal(t, "mysql.service", grants[0].Service)
	assert.Equal(t, []interp.SystemServiceAction{interp.SystemServiceAllActions}, grants[0].Actions)
}

func TestParseAllowedServicesPreservesEntriesForPolicyValidation(t *testing.T) {
	grants := parseAllowedServices("ignored.service,tenant:mysql.service:read,mysql.service:read+freeze")
	require.Len(t, grants, 3)
	assert.Equal(t, interp.SystemdControlGrant{Service: "ignored.service"}, grants[0])
	assert.Equal(t, interp.SystemdControlGrant{
		Service: "tenant:mysql.service",
		Actions: []interp.SystemServiceAction{interp.SystemServiceRead},
	}, grants[1])
	assert.Equal(t, interp.SystemdControlGrant{
		Service: "mysql.service",
		Actions: []interp.SystemServiceAction{interp.SystemServiceRead, "freeze"},
	}, grants[2])
}

func TestAllowedServicesFlagAcceptsServiceGrants(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"--allow-all-commands",
		"--allowed-services", "mysql.service:read+restart,backup.timer:start+stop,api.socket:read+enable+disable,systemd-journald.service:read+clean",
		"--mode", "remediation",
		"-c", `echo hello`,
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
	assert.Empty(t, stderr)
}

func TestAllowedServicesFlagWarnsAndSkipsColonInSelector(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"--allow-all-commands",
		"--allowed-services", "tenant:mysql.service:read",
		"-c", `echo hello`,
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
	assert.Contains(t, stderr, "AllowedSystemServices: skipping")
	assert.Contains(t, stderr, "must not contain ':'")
}

func TestSystemdTargetFlags(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runCLI(t,
		"--allow-all-commands",
		"--systemd-journal-dirs", filepath.Join(dir, "journal"),
		"--systemd-machine-id-path", filepath.Join(dir, "machine-id"),
		"--systemd-journal-socket", filepath.Join(dir, "journal.sock"),
		"--systemd-manager-socket", filepath.Join(dir, "system-bus.sock"),
		"-c", `echo hello`,
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
	assert.Empty(t, stderr)
}

func TestExplicitSystemdTargetRequiresMachineIDPath(t *testing.T) {
	code, _, stderr := runCLI(t,
		"--allow-all-commands",
		"--systemd-journal-dirs", "/host/var/log/journal,/host/run/log/journal",
		"-c", `echo hello`,
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "machine ID path is required")
}

func TestExplicitSystemdManagerTargetRequiresMachineIDPath(t *testing.T) {
	code, _, stderr := runCLI(t,
		"--allow-all-commands",
		"--systemd-manager-socket", "/host/run/dbus/system_bus_socket",
		"-c", `echo hello`,
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "machine ID path is required")
}

func TestAllowAllCommandsFlag(t *testing.T) {
	code, stdout, _ := runCLI(t, "--allow-all-commands", "-c", `echo hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestCommandLongFormRejected(t *testing.T) {
	code, _, stderr := runCLI(t, "--command", "echo hi")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "unknown flag: --command")
}

func TestTimeoutFlagTimesOutExecution(t *testing.T) {
	// Feed a blocking stdin with no -c flag so the timeout fires while readAllContext
	// is waiting for EOF. 50ms is well above Windows' ~15ms timer resolution.
	pr, pw := io.Pipe()
	defer pw.Close()
	var out, errBuf bytes.Buffer
	code := run(context.Background(), []string{"--timeout", "50ms"}, pr, &out, &errBuf)
	assert.Equal(t, exitCodeTimeout, code)
	assert.Contains(t, errBuf.String(), "execution timed out")
}

func TestCanceledContextExitsWithTimeoutCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before execution starts
	code, _, stderr := runCLIContext(t, ctx, "--allow-all-commands", "-c", `echo hello`)
	assert.Equal(t, exitCodeTimeout, code)
	assert.Contains(t, stderr, "execution canceled")
}

func TestTimeoutFlagRejectsNegative(t *testing.T) {
	code, _, stderr := runCLI(t, "--timeout", "-1s", "-c", `echo hello`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--timeout must be >= 0")
}

func TestProcPathFlagInHelp(t *testing.T) {
	code, stdout, _ := runCLI(t, "--help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "--proc-path")
}

// TestScriptExceedsMaxScriptBytes verifies that the CLI rejects a script
// larger than interp.MaxScriptBytes (5 MiB) with exit code 2 and a
// descriptive error message that tells the caller what limit was hit.
func TestScriptExceedsMaxScriptBytes(t *testing.T) {
	// Build a syntactically valid script just over the limit so the error is
	// definitely from the size check, not the parser.
	line := "echo hello\n"
	script := strings.Repeat(line, interp.MaxScriptBytes/len(line)+1)
	require.Greater(t, len(script), interp.MaxScriptBytes)

	code, _, stderr := runCLI(t, "-c", script)
	assert.Equal(t, 2, code, "oversized script should return exit code 2")
	assert.Contains(t, stderr, "exceeds maximum")
	assert.Contains(t, stderr, "5 MiB")
}

// TestScriptAtMaxScriptBytes verifies that a script exactly at the limit is
// accepted (boundary condition).
func TestScriptAtMaxScriptBytes(t *testing.T) {
	// Build a script of exactly MaxScriptBytes: a comment line followed by the
	// command. The comment must end with \n so the parser sees two separate
	// lines, not a single comment that swallows the command.
	cmd := "echo ok\n"
	// comment = (MaxScriptBytes - len(cmd) - 1) '#' chars + '\n'
	comment := strings.Repeat("#", interp.MaxScriptBytes-len(cmd)-1) + "\n"
	script := comment + cmd
	require.Equal(t, interp.MaxScriptBytes, len(script))

	code, stdout, _ := runCLI(t, "--allow-all-commands", "-c", script)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ok\n", stdout)
}
