// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"sort"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Regression coverage for deeply nested scripts. A sub-MaxScriptBytes but
// deeply nested script could drive the recursive-descent parser, the recursive
// validateNode walk, or the recursive executor past the Go runtime's 1 GiB
// goroutine-stack limit, aborting the embedding process with an uncatchable
// "fatal error: stack overflow". ParseScript bounds the parser's call depth
// while it runs and the AST depth after it; validateNode bounds the AST depth
// of scripts parsed elsewhere.

// deepNestingScripts returns, for a given per-level repetition count, a set of
// named scripts that each nest one construct to that depth.
func deepNestingScripts(n int) map[string]string {
	r := strings.Repeat
	parens := r("(", n) + "x" + r(")", n) + "\n"
	return map[string]string{
		"subshell":           r("(", n) + "x" + r(")", n),
		"cmd_substitution":   r("$(", n) + ":" + r(")", n),
		"cmd_subst_in_quote": `"` + r("$(", n) + ":" + r(")", n) + `"`,
		"brace_group":        r("{ ", n) + "x;" + r(" }", n),
		"function_body":      r("f() { ", n) + "x;" + r(" }", n),
		"param_expansion":    "echo " + r("${a:-", n) + "b" + r("}", n),
		"arithmetic":         "echo $((" + r("(", n) + "1" + r(")", n) + "))",
		"pipeline":           r("x|", n) + "x",
		"logical_and":        r("x&&", n) + "x",
		"nested_if":          r("if x; then ", n) + "x" + r("; fi", n),
		"nested_elif":        "if x; then x;" + r("elif x; then x;", n) + "fi",
		"nested_while":       r("while x; do ", n) + "x" + r("; done", n),
		"nested_for":         r("for a in b; do ", n) + "x" + r("; done", n),
		"nested_case":        r("case a in a) ", n) + "x" + r(";; esac", n),
		"if_inside_quote":    `"$(` + r("if x; then ", n) + "x" + r("; fi", n) + `)"`,
		// Operators and prefixes the parser nests without a closing token.
		"arith_ternary":     "echo $((" + r("1?", n) + "1" + r(":1", n) + "))",
		"arith_not":         "echo $((" + r("!", n) + "1))",
		"arith_assign":      "echo $((" + r("a=", n) + "1))",
		"arith_power":       "echo $((" + r("2**", n) + "1))",
		"array_index":       "echo ${a[" + r("b[", n) + "1" + r("]", n) + "]}",
		"slice_offset":      "echo ${a:" + r("1?", n) + "1" + r(":1", n) + "}",
		"assign_index":      "x[" + r("a[", n) + "1" + r("]", n) + "]=1",
		"named_fd_index":    "{fd}>f x[" + r("a[", n) + "1" + r("]", n) + "]=1",
		"test_parens":       "[[ " + r("( ", n) + "a" + r(" )", n) + " ]]",
		"test_not":          "[[ " + r("! ", n) + "a ]]",
		"test_and":          "[[ " + r("a && ", n) + "a ]]",
		"time_prefix":       r("time ", n) + "x",
		"function_prefixes": r("f() ", n) + "{ x; }",
		// Nesting beside text that looks like closers or heredocs but is not.
		"after_ansi_c_heredoc_delim": "cat <<$'E'\nbody\nE\n" + parens,
		"after_empty_heredoc_delim":  "cat <<''\nbody\n\n" + parens,
		"after_arith_shift":          "echo $((1<<2))\n" + parens,
		"after_crlf_heredoc":         "cat <<E\r\nE\r\n" + parens,
		"quoted_heredoc_closers":     r("$(", n) + " cat <<'E'\n" + r(")", 4000) + "\nE\n" + r(")", n),
		"argument_keyword_closers":   r("if x; then ", n) + "echo " + r("fi ", 4000) + "\n" + r("fi\n", n),
		"case_in_case_body":          r("case a in x) case b in y) ", n) + ":" + r(";; esac;; esac", n),
		"negated_if":                 r("! if x; then ", n) + ":" + r("; fi", n),
		"line_continuation":          r("if x; then \\\n", n) + ":" + r("; fi", n),
		"function_keyword":           r("function f { ", n) + ":" + r("; }", n),
		"coproc_named":               r("coproc c { ", n) + ":" + r("; }", n),
	}
}

func isNestingError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "script nesting exceeds")
}

func sortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestParseScriptRejectsDeepNesting checks that ParseScript itself rejects
// every deep shape, at a depth above MaxNestingDepth but far below the depth
// that overflows the stack, so callers that walk the AST before Run are
// covered too.
func TestParseScriptRejectsDeepNesting(t *testing.T) {
	scripts := deepNestingScripts(MaxNestingDepth + 1000)
	for _, name := range sortedNames(scripts) {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseScript(scripts[name], ""); !isNestingError(err) {
				t.Fatalf("ParseScript did not reject the script with a nesting error: %v", err)
			}
		})
	}
}

// TestRunRejectsDeepASTFromOtherParsers checks that validateNode bounds an AST
// that did not come from ParseScript.
func TestRunRejectsDeepASTFromOtherParsers(t *testing.T) {
	for name, script := range map[string]string{
		"pipeline":    strings.Repeat("x|", MaxNestingDepth+1000) + "x",
		"logical_and": strings.Repeat("x&&", MaxNestingDepth+1000) + "x",
	} {
		t.Run(name, func(t *testing.T) {
			prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			var stdout, stderr bytes.Buffer
			runner, err := New(StdIO(nil, &stdout, &stderr))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err := runner.Run(context.Background(), prog); err == nil {
				t.Fatalf("Run accepted a deeply nested AST (stderr=%q)", stderr.String())
			}
			if !strings.Contains(stderr.String(), "nesting exceeds maximum depth") {
				t.Fatalf("Run did not report the nesting limit; stderr=%q", stderr.String())
			}
		})
	}
}

// TestNestingGuardAllowsRealisticScripts guards against false positives:
// scripts with ordinary, long, or unusually deep nesting must be accepted.
func TestNestingGuardAllowsRealisticScripts(t *testing.T) {
	r := strings.Repeat
	cases := map[string]string{
		"flat":              "echo one\necho two\necho three\n",
		"nested_cmd_subst":  `echo "$(echo "$(echo "$(echo deep)")")"`,
		"nested_if":         "if true; then if true; then if true; then echo x; fi; fi; fi",
		"for_loop":          "for i in a b c; do echo $i; done",
		"subshell_group":    "( ( ( echo x ) ) )",
		"and_or":            "true && echo a || echo b",
		"quoted_parens":     `echo "lots of ((((((((( parens ))))))))) in a string"`,
		"comment_parens":    "echo ok # (((((((((( this is only a comment ))))))))))",
		"single_quoted":     `echo '((((((((((((((((((((((((((('`,
		"moderate_subshell": r("( ", 300) + "echo ok" + r(" )", 300),
		"moderate_subst":    r("$(", 200) + "echo ok" + r(")", 200),
		"moderate_arith":    "echo $((" + r("(", 100) + "1" + r(")", 100) + "))",
		"long_pipeline":     "echo start" + r(" | cat", 800),
		"real_world":        "for f in a b c; do\n  if [ -n \"$f\" ]; then\n    echo \"item $f ($(basename \"$f\"))\"\n  fi\ndone\ncase \"$1\" in start) echo go;; *) echo no;; esac\n",
		// Literal text that looks like nesting but is data to the parser.
		"literal_parens_in_quoted_heredoc": "cat <<'E'\n" + r("(", 100000) + "\nE\n",
		"literal_parens_in_single_quotes":  "echo '" + r("(", 100000) + "'",
		"escaped_parens":                   "echo " + r("\\(", 100000),
		"parens_in_comment":                "# " + r("(", 100000),
		"keywords_in_double_quotes":        r("echo \"waiting for work to do if ready while case\"\n", 8000),
		"keyword_looking_arguments":        r("echo if fi done esac }\n", 8000),
		"function_definitions":             r("f() { echo hi; }\n", 4000),
		"normal_heredoc_with_expansion":    "cat <<EOF\nvalue is $(echo x) today\nEOF\necho done\n",
		// Long but flat: the parser builds these iteratively.
		"flat_arithmetic":  "echo $((1" + r("+1", MaxNestingDepth-100) + "))",
		"flat_let":         "let" + r(" a=1", 10000),
		"many_statements":  r("echo hello world; ", 200000),
		"heredoc_big_body": "cat <<E\n" + r("line of text\n", 200000) + "E\n",
		// Syntax a lexical model of the parser has to special-case.
		"comment_ending_in_backslash": "echo hi # copy to C:\\temp\\\necho next\n",
		"nested_backquotes":           "echo `echo \\`echo x\\``",
		"ansi_c_heredoc_delim":        "cat <<$'E'\nbody\nE\necho after\n",
		"named_fd_redirect":           "{fd}>f echo hi",
		"arith_shift":                 "echo $((1 << 2))\nlet 'x=1<<2'\na[1<<2]=3\n",
		"crlf_heredoc":                "cat <<E\r\n(((\r\nE\r\n",
		"case_patterns":               r("case a in (a) { echo; };; (b|c) :;; esac\n", 2000),
	}
	for _, name := range sortedNames(cases) {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseScript(cases[name], ""); err != nil {
				t.Fatalf("ParseScript rejected a valid script: %v", err)
			}
		})
	}
}

// TestParseScriptDepthIsRelativeToCaller checks that the parser's depth bound
// does not count the caller's own stack, so a moderately nested script still
// parses when ParseScript is called from deep inside an embedding program.
func TestParseScriptDepthIsRelativeToCaller(t *testing.T) {
	script := strings.Repeat("$(", 200) + "echo ok" + strings.Repeat(")", 200)
	var err error
	var recurse func(n int)
	recurse = func(n int) {
		if n == 0 {
			_, err = ParseScript(script, "")
			return
		}
		recurse(n - 1)
	}
	recurse(50000)
	if err != nil {
		t.Fatalf("ParseScript rejected a moderately nested script from a deep caller: %v", err)
	}
}

// --- subprocess crash tests -------------------------------------------------

const deepNestingChildEnv = "RSHELL_DEEP_NESTING_CHILD"

// TestDeepNestingDoesNotCrashProcess feeds every deep shape, at a depth far
// past the stack-overflow threshold of an unbounded parser, through
// ParseScript and Run in a re-exec child whose goroutine stack is capped at
// 8 MiB. An uncatchable "fatal error: stack overflow" is therefore observed as
// a failed child rather than aborting this test binary.
func TestDeepNestingDoesNotCrashProcess(t *testing.T) {
	if os.Getenv(deepNestingChildEnv) == "1" {
		deepNestingChild()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDeepNestingDoesNotCrashProcess$", "-test.v")
	cmd.Env = append(os.Environ(), deepNestingChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if bytes.Contains(out, []byte("stack overflow")) {
		t.Fatalf("deeply nested script crashed the process with a stack overflow:\n%s", out)
	}
	if err != nil {
		t.Fatalf("deeply nested script crashed the child process: %v\n%s", err, out)
	}
}

// deepNestingChild exits 0 only if every deep script was rejected without a
// fatal runtime throw.
func deepNestingChild() {
	debug.SetMaxStack(8 << 20)
	for _, depth := range []int{60000, 300000} {
		scripts := deepNestingScripts(depth)
		for _, name := range sortedNames(scripts) {
			prog, err := ParseScript(scripts[name], "")
			if err != nil {
				continue
			}
			var stdout, stderr bytes.Buffer
			runner, nerr := New(StdIO(nil, &stdout, &stderr))
			if nerr != nil {
				fmt.Printf("child: New failed for %s: %v\n", name, nerr)
				os.Exit(3)
			}
			if rerr := runner.Run(context.Background(), prog); rerr == nil {
				fmt.Printf("child: %s at depth %d was accepted\n", name, depth)
				os.Exit(3)
			}
		}
	}
	fmt.Println("child: all deep scripts handled without crashing")
	os.Exit(0)
}
