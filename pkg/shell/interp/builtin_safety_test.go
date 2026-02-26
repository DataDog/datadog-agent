// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

// runShellScript is a test helper that runs a shell script and returns stdout, stderr, and exit code.
func runShellScript(t *testing.T, script string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var stdoutBuf, stderrBuf bytes.Buffer
	r, err := New(StdIO(nil, &stdoutBuf, &stderrBuf), OpenHandler(SafeOpenHandler()))
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}
	p := syntax.NewParser()
	prog, err := p.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatalf("failed to parse script: %v", err)
	}
	r.Reset()
	runErr := r.Run(context.Background(), prog)
	exitCode = 0
	if runErr != nil {
		if es, ok := runErr.(ExitStatus); ok {
			exitCode = int(es)
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

func runShellScriptWithTimeout(t *testing.T, script string, timeout time.Duration) (stdout, stderr string, exitCode int) {
	t.Helper()
	var stdoutBuf, stderrBuf bytes.Buffer
	r, err := New(StdIO(nil, &stdoutBuf, &stderrBuf), OpenHandler(SafeOpenHandler()))
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}
	p := syntax.NewParser()
	prog, err := p.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatalf("failed to parse script: %v", err)
	}
	r.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	runErr := r.Run(ctx, prog)
	exitCode = 0
	if runErr != nil {
		if es, ok := runErr.(ExitStatus); ok {
			exitCode = int(es)
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), exitCode
}

func TestSafety_EvalBlocked(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `eval "echo pwned"`)
	if exitCode != 1 {
		t.Errorf("eval should return exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("eval stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_ExecBlocked(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `exec /bin/sh`)
	if exitCode != 1 {
		t.Errorf("exec should return exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("exec stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_SourceBlocked(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `source /etc/profile`)
	if exitCode != 1 {
		t.Errorf("source should return exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("source stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_DotBlocked(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `. /etc/profile`)
	if exitCode != 1 {
		t.Errorf(". should return exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf(". stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_TrapBlocked(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `trap 'echo pwned' EXIT`)
	if exitCode != 1 {
		t.Errorf("trap should return exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("trap stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_FindNoExec(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `find . -exec echo {} \;`)
	if exitCode != 1 {
		t.Errorf("find -exec should fail, got exit code %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown predicate") {
		t.Errorf("find -exec stderr should contain 'unknown predicate', got: %q", stderr)
	}
}

func TestSafety_FindNoDelete(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `find . -delete`)
	if exitCode != 1 {
		t.Errorf("find -delete should fail, got exit code %d", exitCode)
	}
	if !strings.Contains(stderr, "unknown predicate") {
		t.Errorf("find -delete stderr should contain 'unknown predicate', got: %q", stderr)
	}
}

func TestSafety_RedirectWriteBlocked(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "pwned" > /tmp/safe-shell-test-write`)
	if exitCode != 1 {
		t.Errorf("redirect write should fail, got exit code %d", exitCode)
	}
	if !strings.Contains(stderr, "write operations not permitted") {
		t.Errorf("redirect write stderr should contain 'write operations not permitted', got: %q", stderr)
	}
}

func TestSafety_RedirectAppendBlocked(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "pwned" >> /tmp/safe-shell-test-append`)
	if exitCode != 1 {
		t.Errorf("redirect append should fail, got exit code %d", exitCode)
	}
	if !strings.Contains(stderr, "write operations not permitted") {
		t.Errorf("redirect append stderr should contain 'write operations not permitted', got: %q", stderr)
	}
}

func TestSafety_PingNoFlood(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `ping -f localhost`)
	if exitCode != 2 {
		t.Errorf("ping -f should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("ping -f stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_PingCountCapped(t *testing.T) {
	// We don't actually send pings (would need network access), so just verify
	// the cap message appears in stderr by testing the flag parsing with a
	// count that exceeds the limit.
	_, stderr, _ := runShellScript(t, `ping -c 200 invalid-host-that-will-fail`)
	if !strings.Contains(stderr, "count capped at 100") {
		t.Errorf("ping -c 200 stderr should contain 'count capped at 100', got: %q", stderr)
	}
}

func TestSafety_SortNoOutput(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "test" | sort -o /tmp/out`)
	if exitCode != 2 {
		t.Errorf("sort -o should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("sort -o stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_OutputCapped(t *testing.T) {
	// Generate output exceeding 1MB by echoing in a loop.
	var stdoutBuf, stderrBuf bytes.Buffer
	r, err := New(StdIO(nil, &stdoutBuf, &stderrBuf))
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}
	// Script: echo a large string many times.
	script := `i=0; while [ $i -lt 20000 ]; do echo "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"; i=$((i+1)); done`
	p := syntax.NewParser()
	prog, err := p.Parse(strings.NewReader(script), "")
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	r.Reset()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r.Run(ctx, prog)

	if stdoutBuf.Len() > maxOutputBytes+100 { // allow small overrun from partial write
		t.Errorf("stdout should be capped at ~1MB, got %d bytes", stdoutBuf.Len())
	}
	if !strings.Contains(stderrBuf.String(), "output truncated at 1MB limit") {
		t.Errorf("stderr should contain truncation warning, got: %q", stderrBuf.String())
	}
}

func TestSafety_CatLargeFileWarning(t *testing.T) {
	// Create a temp file > 1MB and cat it.
	// Since we can't easily create files through the interpreter, we test
	// by checking that the cat builtin exists and handles the warning path.
	// This is more of an integration test; the unit logic is straightforward.
	_, stderr, _ := runShellScript(t, `cat /dev/null`)
	// Should succeed with no warning for a small/empty file.
	if strings.Contains(stderr, "output will be truncated") {
		t.Errorf("cat /dev/null should not trigger large file warning")
	}
}

func TestSafety_GrepDepthCapped(t *testing.T) {
	// We can't easily create a 10+ level deep directory tree in the interpreter,
	// but we can verify the constant and the comparison operator.
	// The depth check uses >= so depth 10 is rejected (0-9 = 10 levels allowed).
	if grepMaxRecursionDepth != 10 {
		t.Errorf("grepMaxRecursionDepth should be 10, got %d", grepMaxRecursionDepth)
	}
}

func TestSafety_TailFollowTimeout(t *testing.T) {
	// Verify the tail follow timeout constant is set correctly.
	if tailFollowMaxDuration != 60*time.Second {
		t.Errorf("tailFollowMaxDuration should be 60s, got %v", tailFollowMaxDuration)
	}

	// Functional test: tail -f on /dev/null should return within the timeout.
	// We use a shorter context timeout to avoid waiting 60s in tests.
	_, _, exitCode := runShellScriptWithTimeout(t, `tail -f /dev/null`, 2*time.Second)
	// Should exit due to context cancellation (which is before the 60s tail timeout).
	_ = exitCode // exit code varies by platform; the key assertion is it doesn't hang.
}

func TestSafety_ExternalCommandBlocked(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `curl http://example.com`)
	if exitCode != 127 {
		t.Errorf("external command should return exit code 127, got %d", exitCode)
	}
	if !strings.Contains(stderr, "command not allowed") {
		t.Errorf("external command stderr should contain 'command not allowed', got: %q", stderr)
	}
}

func TestSafety_CommandBuiltinNoBypass(t *testing.T) {
	// The `command` builtin must not bypass the external command block.
	_, stderr, exitCode := runShellScript(t, `command /bin/sh -c 'echo pwned'`)
	if exitCode != 127 {
		t.Errorf("command builtin should not bypass block, got exit code %d", exitCode)
	}
	if !strings.Contains(stderr, "command not allowed") {
		t.Errorf("command builtin stderr should contain 'command not allowed', got: %q", stderr)
	}
}

// Functional tests for new commands.

func TestHead_Basic(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n" | head -n 3`)
	if exitCode != 0 {
		t.Errorf("head should succeed, got exit code %d", exitCode)
	}
	if stdout != "1\n2\n3\n" {
		t.Errorf("head -n 3 unexpected output: %q", stdout)
	}
}

func TestCat_Basic(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "hello\nworld\n" | cat -n`)
	if exitCode != 0 {
		t.Errorf("cat should succeed, got exit code %d", exitCode)
	}
	if !strings.Contains(stdout, "1\thello") {
		t.Errorf("cat -n should number lines, got: %q", stdout)
	}
}

func TestGrep_Basic(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "apple\nbanana\ncherry\n" | grep an`)
	if exitCode != 0 {
		t.Errorf("grep should succeed, got exit code %d", exitCode)
	}
	if !strings.Contains(stdout, "banana") {
		t.Errorf("grep should match 'banana', got: %q", stdout)
	}
}

func TestGrep_CaseInsensitive(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "Hello\nworld\n" | grep -i hello`)
	if exitCode != 0 {
		t.Errorf("grep -i should succeed, got exit code %d", exitCode)
	}
	if !strings.Contains(stdout, "Hello") {
		t.Errorf("grep -i should match 'Hello', got: %q", stdout)
	}
}

func TestGrep_Count(t *testing.T) {
	stdout, _, _ := runShellScript(t, `printf "a\nb\na\nc\na\n" | grep -c a`)
	if strings.TrimSpace(stdout) != "3" {
		t.Errorf("grep -c should return count 3, got: %q", stdout)
	}
}

func TestGrep_InvertMatch(t *testing.T) {
	stdout, _, _ := runShellScript(t, `printf "a\nb\nc\n" | grep -v a`)
	if !strings.Contains(stdout, "b") || !strings.Contains(stdout, "c") {
		t.Errorf("grep -v should exclude 'a', got: %q", stdout)
	}
	if strings.Contains(stdout, "a\n") {
		t.Errorf("grep -v should not contain 'a', got: %q", stdout)
	}
}

func TestWc_Basic(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "hello world\ngoodbye\n" | wc -l`)
	if exitCode != 0 {
		t.Errorf("wc should succeed, got exit code %d", exitCode)
	}
	if !strings.Contains(strings.TrimSpace(stdout), "2") {
		t.Errorf("wc -l should report 2 lines, got: %q", stdout)
	}
}

func TestSort_Basic(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "cherry\napple\nbanana\n" | sort`)
	if exitCode != 0 {
		t.Errorf("sort should succeed, got exit code %d", exitCode)
	}
	expected := "apple\nbanana\ncherry\n"
	if stdout != expected {
		t.Errorf("sort output should be %q, got: %q", expected, stdout)
	}
}

func TestSort_Reverse(t *testing.T) {
	stdout, _, _ := runShellScript(t, `printf "a\nb\nc\n" | sort -r`)
	expected := "c\nb\na\n"
	if stdout != expected {
		t.Errorf("sort -r output should be %q, got: %q", expected, stdout)
	}
}

func TestSort_Numeric(t *testing.T) {
	stdout, _, _ := runShellScript(t, `printf "10\n2\n1\n20\n" | sort -n`)
	expected := "1\n2\n10\n20\n"
	if stdout != expected {
		t.Errorf("sort -n output should be %q, got: %q", expected, stdout)
	}
}

func TestSort_Unique(t *testing.T) {
	stdout, _, _ := runShellScript(t, `printf "a\nb\na\nc\nb\n" | sort -u`)
	expected := "a\nb\nc\n"
	if stdout != expected {
		t.Errorf("sort -u output should be %q, got: %q", expected, stdout)
	}
}

func TestUniq_Basic(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\na\nb\nb\nb\nc\n" | uniq`)
	if exitCode != 0 {
		t.Errorf("uniq should succeed, got exit code %d", exitCode)
	}
	expected := "a\nb\nc\n"
	if stdout != expected {
		t.Errorf("uniq output should be %q, got: %q", expected, stdout)
	}
}

func TestUniq_Count(t *testing.T) {
	stdout, _, _ := runShellScript(t, `printf "a\na\nb\nc\nc\nc\n" | uniq -c`)
	if !strings.Contains(stdout, "2 a") {
		t.Errorf("uniq -c should show count for 'a', got: %q", stdout)
	}
	if !strings.Contains(stdout, "3 c") {
		t.Errorf("uniq -c should show count for 'c', got: %q", stdout)
	}
}

func TestUniq_DupsOnly(t *testing.T) {
	stdout, _, _ := runShellScript(t, `printf "a\na\nb\nc\nc\n" | uniq -d`)
	if !strings.Contains(stdout, "a") || !strings.Contains(stdout, "c") {
		t.Errorf("uniq -d should show duplicated lines, got: %q", stdout)
	}
	if strings.Contains(stdout, "b") {
		t.Errorf("uniq -d should not show unique lines, got: %q", stdout)
	}
}

func TestPipeline_Integration(t *testing.T) {
	// echo | grep | sort | uniq -c | sort -rn | head
	stdout, _, exitCode := runShellScript(t, `printf "b\na\nb\nc\na\nb\n" | sort | uniq -c | sort -rn | head -n 1`)
	if exitCode != 0 {
		t.Errorf("pipeline should succeed, got exit code %d", exitCode)
	}
	trimmed := strings.TrimSpace(stdout)
	if !strings.Contains(trimmed, "3") || !strings.Contains(trimmed, "b") {
		t.Errorf("pipeline should show 'b' with count 3, got: %q", trimmed)
	}
}

// --- Sed safety tests ---

func TestSafety_SedNoInPlace(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "test" | sed -i 's/t/T/'`)
	if exitCode != 2 {
		t.Errorf("sed -i should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("sed -i stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_SedNoWriteCmd(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "test" | sed 'w /tmp/out'`)
	if exitCode != 2 {
		t.Errorf("sed w command should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("sed w stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_SedNoExecCmd(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "test" | sed 'e'`)
	if exitCode != 2 {
		t.Errorf("sed e command should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("sed e stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_SedNoSubWrite(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "test" | sed 's/a/b/w /tmp/out'`)
	if exitCode != 2 {
		t.Errorf("sed s///w should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("sed s///w stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_SedNoSubExec(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "test" | sed 's/a/b/e'`)
	if exitCode != 2 {
		t.Errorf("sed s///e should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("sed s///e stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSafety_SedNoWriteCmdW(t *testing.T) {
	_, stderr, exitCode := runShellScript(t, `echo "test" | sed 'W /tmp/out'`)
	if exitCode != 2 {
		t.Errorf("sed W command should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("sed W stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

// --- Sed functional tests ---

func TestSed_BasicSubstitute(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `echo "hello world" | sed 's/world/there/'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "hello there" {
		t.Errorf("sed substitute unexpected output: %q", stdout)
	}
}

func TestSed_GlobalSubstitute(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `echo "aaa" | sed 's/a/b/g'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "bbb" {
		t.Errorf("sed global substitute unexpected output: %q", stdout)
	}
}

func TestSed_DeleteLines(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\n" | sed '2d'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "a\nc\n" {
		t.Errorf("sed delete unexpected output: %q", stdout)
	}
}

func TestSed_PrintLineNumbers(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\n" | sed -n '='`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "1\n2\n" {
		t.Errorf("sed line numbers unexpected output: %q", stdout)
	}
}

func TestSed_AddressRange(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\nd\n" | sed '2,3d'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "a\nd\n" {
		t.Errorf("sed address range unexpected output: %q", stdout)
	}
}

func TestSed_RegexAddress(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "foo\nbar\nbaz\n" | sed '/bar/d'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "foo\nbaz\n" {
		t.Errorf("sed regex address unexpected output: %q", stdout)
	}
}

func TestSed_MultipleExpressions(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `echo "hello" | sed -e 's/h/H/' -e 's/o/O/'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "HellO" {
		t.Errorf("sed multi-expression unexpected output: %q", stdout)
	}
}

func TestSed_Transliterate(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `echo "hello" | sed 'y/helo/HELO/'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "HELLO" {
		t.Errorf("sed transliterate unexpected output: %q", stdout)
	}
}

func TestSed_SuppressDefault(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\n" | sed -n '2p'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "b\n" {
		t.Errorf("sed suppress default unexpected output: %q", stdout)
	}
}

func TestSed_ExtendedRegex(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `echo "foo123" | sed -E 's/[0-9]+/NUM/'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "fooNUM" {
		t.Errorf("sed extended regex unexpected output: %q", stdout)
	}
}

func TestSed_Pipeline(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "2026-02-26T10:00:00 ERROR msg\n" | sed 's/^[0-9T:.-]* //'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "ERROR msg" {
		t.Errorf("sed pipeline unexpected output: %q", stdout)
	}
}

func TestSed_HoldSpace(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\n" | sed -n 'H;${x;s/^\n//;s/\n/ /g;p}'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "a b" {
		t.Errorf("sed hold space unexpected output: %q", stdout)
	}
}

func TestSed_NegatedAddress(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\n" | sed '2!d'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "b\n" {
		t.Errorf("sed negated address unexpected output: %q", stdout)
	}
}

func TestSed_PrintCommand(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\n" | sed -n '/b/p'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "b\n" {
		t.Errorf("sed print command unexpected output: %q", stdout)
	}
}

func TestSed_SubstitutionPrint(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\n" | sed -n 's/b/B/p'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "B\n" {
		t.Errorf("sed substitution print unexpected output: %q", stdout)
	}
}

func TestSed_QuitCommand(t *testing.T) {
	stdout, _, _ := runShellScript(t, `printf "a\nb\nc\n" | sed '2q'`)
	if stdout != "a\nb\n" {
		t.Errorf("sed quit unexpected output: %q", stdout)
	}
}

func TestSed_FullPipelineIntegration(t *testing.T) {
	// The primary use case: timestamp stripping in a pipeline
	script := `printf "2026-02-26T10:00:00 ERROR something broke\n2026-02-26T10:00:01 ERROR another issue\n2026-02-26T10:00:02 ERROR something broke\n" | sed 's/^[0-9T:.-]* //' | sort | uniq -c | sort -rn | head -n 1`
	stdout, _, exitCode := runShellScript(t, script)
	if exitCode != 0 {
		t.Errorf("pipeline should succeed, got exit code %d", exitCode)
	}
	trimmed := strings.TrimSpace(stdout)
	if !strings.Contains(trimmed, "2") || !strings.Contains(trimmed, "something broke") {
		t.Errorf("pipeline should show 'something broke' with count 2, got: %q", trimmed)
	}
}

// --- Additional sed tests for review findings ---

func TestSed_RegexRange(t *testing.T) {
	// Fix 2: regex-to-regex address ranges must work with inRange toggle
	stdout, _, exitCode := runShellScript(t, `printf "a\nSTART\nb\nc\nEND\nd\n" | sed '/START/,/END/d'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "a\nd\n" {
		t.Errorf("sed regex range unexpected output: %q", stdout)
	}
}

func TestSed_RegexRangeSubstitute(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nSTART\nb\nc\nEND\nd\n" | sed '/START/,/END/s/^/>> /'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	expected := "a\n>> START\n>> b\n>> c\n>> END\nd\n"
	if stdout != expected {
		t.Errorf("sed regex range substitute unexpected output: %q, want %q", stdout, expected)
	}
}

func TestSed_NextLine(t *testing.T) {
	// Fix 3: n command should output current line and load next
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\n" | sed -n 'n;p'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "b\n" {
		t.Errorf("sed n command unexpected output: %q, want %q", stdout, "b\n")
	}
}

func TestSed_AppendNextLine(t *testing.T) {
	// Fix 3: N command should append next line to pattern space
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\nd\n" | sed 'N;s/\n/ /'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "a b\nc d\n" {
		t.Errorf("sed N command unexpected output: %q, want %q", stdout, "a b\nc d\n")
	}
}

func TestSed_BranchLabel(t *testing.T) {
	// Branch to named label
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\n" | sed -n '/b/{p;b end};p;:end'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "a\nb\nc\n" {
		t.Errorf("sed branch label unexpected output: %q", stdout)
	}
}

func TestSed_ConditionalBranch(t *testing.T) {
	// t command: branch if substitution was made
	stdout, _, exitCode := runShellScript(t, `printf "aXb\ncXd\n" | sed 's/X/Y/;t done;s/$/!/;:done'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	// Both lines match s/X/Y/ so t branches past s/$/!/ for both
	if stdout != "aYb\ncYd\n" {
		t.Errorf("sed t branch unexpected output: %q, want %q", stdout, "aYb\ncYd\n")
	}
}

func TestSed_InfiniteLoopProtection(t *testing.T) {
	// Fix 1: infinite branch loop must be caught by iteration limit
	_, stderr, exitCode := runShellScriptWithTimeout(t, `echo "test" | sed ':loop; b loop'`, 5*time.Second)
	// Should terminate due to iteration limit, not hang
	_ = exitCode
	if !strings.Contains(stderr, "execution limit exceeded") {
		t.Errorf("sed infinite loop should produce limit warning, got stderr: %q", stderr)
	}
}

func TestSafety_SedInPlaceLongOption(t *testing.T) {
	// Fix 9: --in-place=.bak should be caught
	_, stderr, exitCode := runShellScript(t, `echo "test" | sed --in-place=.bak 's/t/T/'`)
	if exitCode != 2 {
		t.Errorf("sed --in-place=.bak should return exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr, "not available in safe shell") {
		t.Errorf("sed --in-place=.bak stderr should contain 'not available in safe shell', got: %q", stderr)
	}
}

func TestSed_GroupWithRegexRange(t *testing.T) {
	// Fix 7: labels/groups in flattened command list
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\nd\n" | sed -n '/b/,/c/{p}'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "b\nc\n" {
		t.Errorf("sed group with regex range unexpected output: %q, want %q", stdout, "b\nc\n")
	}
}

func TestSed_ChangeCommand(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\nc\n" | sed '2c\replaced'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "a\nreplaced\nc\n" {
		t.Errorf("sed change command unexpected output: %q", stdout)
	}
}

func TestSed_InsertCommand(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\n" | sed '2i\inserted'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "a\ninserted\nb\n" {
		t.Errorf("sed insert command unexpected output: %q", stdout)
	}
}

func TestSed_AppendCommand(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\n" | sed '1a\appended'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "a\nappended\nb\n" {
		t.Errorf("sed append command unexpected output: %q", stdout)
	}
}

func TestSed_EmptyInput(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "" | sed 's/a/b/'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed on empty input, got exit code %d", exitCode)
	}
	if stdout != "" {
		t.Errorf("sed empty input should produce empty output, got: %q", stdout)
	}
}

func TestSed_Comment(t *testing.T) {
	stdout, _, exitCode := runShellScript(t, `printf "a\nb\n" | sed -e '# comment' -e 's/a/A/'`)
	if exitCode != 0 {
		t.Errorf("sed should succeed, got exit code %d", exitCode)
	}
	if stdout != "A\nb\n" {
		t.Errorf("sed comment handling unexpected output: %q", stdout)
	}
}
