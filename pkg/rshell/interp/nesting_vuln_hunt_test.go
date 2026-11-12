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
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

// Regression coverage for the unbounded-nesting-depth stack-overflow crash
// (VULN-4562 / VULN-1673) and the unbounded-pipeline-depth variant
// (VULN-4535). A sub-MaxScriptBytes but deeply nested script drove the
// recursive-descent parser, the recursive validateNode walk, or the recursive
// executor past the Go runtime's 1 GiB goroutine-stack limit, aborting the
// embedding process with an uncatchable "fatal error: stack overflow" before
// any command, path, mode, or timeout policy was consulted. The fix bounds
// nesting depth with a lexical pre-parse guard (ParseScript) and an iterative
// post-parse guard (validateNode); see MaxNestingDepth.

// deepNestingScripts returns, for a given per-level repetition count, a set of
// named scripts that each nest one construct to that depth. Parser-recursive
// shapes (brackets and substitutions) are caught by the lexical guard; the
// remaining shapes parse successfully but are caught by the AST guard.
func deepNestingScripts(n int) map[string]string {
	r := strings.Repeat
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
	}
}

// TestNestingGuardRejectsDeepScriptsWithoutCrashing checks that moderately
// deep scripts — above MaxNestingDepth but far below the depth that actually
// overflows the stack, so that on unpatched code they would simply parse and
// run rather than crash the test binary — are rejected with a nesting-depth
// error through either ParseScript or Run, never accepted. This fails on main
// (the scripts are accepted) and passes with the fix.
func TestNestingGuardRejectsDeepScriptsWithoutCrashing(t *testing.T) {
	const depth = MaxNestingDepth + 1000
	for name, script := range deepNestingScripts(depth) {
		t.Run(name, func(t *testing.T) {
			prog, err := ParseScript(script, "")
			if err != nil {
				if !strings.Contains(err.Error(), "nesting exceeds maximum depth") {
					t.Fatalf("ParseScript rejected for the wrong reason: %v", err)
				}
				return // rejected at parse time by the lexical guard
			}
			// Parsed successfully: the AST guard in validateNode (reached via
			// Run) must reject it.
			var stdout, stderr bytes.Buffer
			runner, err := New(StdIO(nil, &stdout, &stderr))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err := runner.Run(context.Background(), prog); err == nil {
				t.Fatalf("Run accepted a script nested to depth %d (stderr=%q)", depth, stderr.String())
			}
			if !strings.Contains(stderr.String(), "nesting exceeds maximum depth") {
				t.Fatalf("Run did not report the nesting limit; stderr=%q", stderr.String())
			}
		})
	}
}

// TestNestingGuardAllowsRealisticScripts guards against false positives: scripts
// with ordinary — even unusually deep — nesting must still be accepted.
func TestNestingGuardAllowsRealisticScripts(t *testing.T) {
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
		"moderate_subshell": strings.Repeat("( ", 300) + "echo ok" + strings.Repeat(" )", 300),
		"long_pipeline":     "echo start" + strings.Repeat(" | cat", 800),
		// Keyword-looking words inside quotes must not be counted as compound
		// commands, even repeated across a large script.
		"keywords_in_double_quotes": strings.Repeat("echo \"waiting for work to do if ready while case\"\n", 8000),
		"keywords_in_single_quotes": strings.Repeat("echo 'for do if while until case done fi esac'\n", 8000),
		"keyword_substrings":        strings.Repeat("echo ifconfig doing format cases fortune\n", 8000),
		"brackets_in_double_quotes": strings.Repeat("echo \"json { a: [ ( nested ) ] } done\"\n", 5000),
		"real_world":                "for f in a b c; do\n  if [ -n \"$f\" ]; then\n    echo \"item $f ($(basename \"$f\"))\"\n  fi\ndone\ncase \"$1\" in start) echo go;; *) echo no;; esac\n",
		// Context-tracking false-positive regressions (reported by review of the
		// first version of the lexical guard): literal brackets/keywords that the
		// parser treats as data must not be counted as nesting.
		"literal_parens_in_quoted_heredoc": "cat <<'E'\n" + strings.Repeat("(", 5100) + "\nE\n",
		"keyword_looking_arguments":        strings.Repeat("echo if\n", 5001),
		"closer_looking_arguments":         strings.Repeat("echo fi done esac\n", 5001),
		"brace_looking_arguments":          strings.Repeat("echo }\n", 5001),
		"function_definitions":             strings.Repeat("f() { echo hi; }\n", 4000),
		"normal_heredoc_with_expansion":    "cat <<EOF\nvalue is $(echo x) today\nEOF\necho done\n",
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			prog, err := ParseScript(script, "")
			if err != nil {
				t.Fatalf("ParseScript falsely rejected a realistic script: %v", err)
			}
			if err := checkNestingDepth(prog); err != nil {
				t.Fatalf("AST guard falsely rejected a realistic script: %v", err)
			}
		})
	}
}

// TestLexicalGuardTracksParserContexts proves the pre-parse guard counts the
// same nesting the parser does: literal text that the parser treats as data —
// quoted heredoc bodies and keyword/brace-looking *arguments* — must not clear
// the guard's tracked frames, so a deeply nested script that hides its real
// nesting behind such text is still rejected before the recursive parser runs.
// (Regression for a review finding that the first lexical guard undercounted
// these, letting the parser build the deep AST anyway.)
func TestLexicalGuardTracksParserContexts(t *testing.T) {
	r := strings.Repeat
	const deep = 60000 // far more than MaxNestingDepth, but under MaxScriptBytes

	// Real nested "$(" whose count the parser pays for, with a quoted heredoc
	// body full of ")" that must NOT pop the guard's frames.
	heredocHidden := r("$(", deep) + " cat <<'E'\n" + r(")", 4000) + "\nE\n" + r(")", deep)

	// Real nested "if" whose count the parser pays for, with argument-position
	// "fi" words that must NOT pop the guard's if-frames.
	argKeywordHidden := r("if x; then ", deep) + "echo " + r("fi ", 4000) + "\n" + r("fi\n", deep)

	// Deep "$(" inside an *unquoted* heredoc body still expands, so the parser
	// recurses into it and the guard must count it.
	subInUnquotedHeredoc := "cat <<E\n" + r("$(", deep) + "\n" + r(")", deep) + "\nE\n"

	for name, script := range map[string]string{
		"heredoc_hidden_nesting":   heredocHidden,
		"argument_keyword_nesting": argKeywordHidden,
		"sub_in_unquoted_heredoc":  subInUnquotedHeredoc,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseScript(script, "")
			if err == nil {
				t.Fatalf("deeply nested script was not rejected before parsing")
			}
			if !strings.Contains(err.Error(), "nesting exceeds maximum depth") {
				t.Fatalf("rejected for the wrong reason: %v", err)
			}
		})
	}
}

// lexicalBypassScripts returns scripts whose real parser nesting is deep
// (depth levels) but which hide that nesting behind a construct that an
// earlier version of the lexical guard mis-tokenised — opening a phantom
// heredoc or quote, or leaving command position — so the guard undercounted.
// (Regressions for the second round of review findings on the guard.)
func lexicalBypassScripts(depth int) map[string]string {
	r := strings.Repeat
	parens := r("(", depth) + "x" + r(")", depth) + "\n"
	subs := r("$(", depth) + ":" + r(")", depth) + "\n"
	ifs := r("if x; then ", depth) + ":" + r("; fi", depth) + "\n"
	return map[string]string{
		// "<<" is a shift inside arithmetic, not a heredoc operator.
		"arith_shift":           "echo $((1 << 2))\n" + parens,
		"arith_shift_unspaced":  "$((1<<9))\n" + parens,
		"arith_shift_if":        "echo $((1<<9))\n" + ifs,
		"arith_command_shift":   "((a<<b))\n" + parens,
		"arith_for_shift":       "for ((i=0;i<<2;i++)); do :; done\n" + parens,
		"arith_bracket_shift":   "echo $[1<<2]\n" + parens,
		"arith_in_dquote":       "echo \"$((1<<2))\"\n" + parens,
		"arith_in_heredoc_body": "cat <<E\n$((1<<2))\nE\n" + parens,
		"let_shift":             "let x<<2\n" + parens,
		"index_assign_shift":    "a[1<<2]=3\n" + parens,
		"index_prefix_shift":    "a=1 b[1<<2]=3 true\n" + parens,
		"declare_index_shift":   "declare -a a[1<<2]=3\n" + parens,
		"param_word_shift":      "echo ${x//<</y}\n" + parens,
		// A case pattern's ")" puts the parser in command position.
		"case_in_case_body":  r("case a in x) case b in y) ", depth) + ":" + r(";; esac;; esac", depth),
		"brace_in_case_body": r("case a in x) { ", depth) + ":" + r("; };; esac", depth),
		"case_paren_pattern": r("case a in (x) { ", depth) + ":" + r("; };; esac", depth),
		// The parser closes a heredoc at "E\r\n" in a CRLF script.
		"crlf_heredoc": "cat <<E\r\nE\r\n" + parens,
		// Other positions where a command (and so a reserved word) follows.
		"negated_if":        r("! if x; then ", depth) + ":" + r("; fi", depth),
		"time_if":           r("time -p if x; then ", depth) + ":" + r("; fi", depth),
		"line_continuation": r("if x; then \\\n", depth) + ":" + r("; fi", depth),
		"function_keyword":  r("function f { ", depth) + ":" + r("; }", depth),
		"coproc_named":      r("coproc c { ", depth) + ":" + r("; }", depth),
		// $'\'' is one ANSI-C string; misreading it opens a phantom quote.
		"ansi_c_escaped_quote": "echo $'\\''\n" + subs + "echo \"'\"\n",
	}
}

// TestLexicalGuardRejectsBypassShapes proves each bypass shape is rejected by
// the pre-parse guard itself (ParseScript does not run the AST guard), i.e.
// before the recursive parser could descend into it.
func TestLexicalGuardRejectsBypassShapes(t *testing.T) {
	for name, script := range lexicalBypassScripts(MaxNestingDepth + 100) {
		t.Run(name, func(t *testing.T) {
			_, err := ParseScript(script, "")
			if err == nil || !strings.Contains(err.Error(), "nesting exceeds maximum depth") {
				t.Fatalf("deeply nested script was not rejected before parsing: %v", err)
			}
		})
	}
	// Each prefix is a valid script on its own when not followed by deep
	// nesting, so the rejection above is due to the nesting it hides.
	for name, script := range lexicalBypassScripts(3) {
		t.Run(name+"/shallow", func(t *testing.T) {
			if err := checkScriptNestingDepth(script); err != nil {
				t.Fatalf("shallow variant falsely rejected: %v", err)
			}
		})
	}
}

// TestLexicalGuardAllowsKeywordLookingWords guards against the false positives
// that context tracking must avoid: words that look like reserved words or
// brackets but are arguments, patterns, loop lists, or arithmetic.
func TestLexicalGuardAllowsKeywordLookingWords(t *testing.T) {
	r := strings.Repeat
	cases := map[string]string{
		"in_command_after_then":    r("if true; then in if; :; fi\n", 6000),
		"keywords_in_for_list":     r("for x in if while case do; do :; done\n", 6000),
		"keywords_in_case_pattern": r("case x in if|while) :;; esac\n", 6000),
		"case_paren_pattern":       r("case a in (a) { echo; };; (b|c) :;; esac\n", 6000),
		"case_fallthrough":         r("case a in a) :;& b) :;;& *) ;; esac\n", 6000),
		"shift_operators":          r("echo $((1<<2)) $[3<<1]\nlet 'x=1<<2' y<<=1\n((z<<=1))\na[1<<2]=3\n", 3000),
		"literal_parens_in_param":  r("echo ${x:-(} ${x/(/[} ${x:-{}}\n", 6000),
		"crlf_heredoc":             r("cat <<E\r\n(((\r\nE\r\n", 6000),
		"negation":                 r("! true\n", 6000),
		"else_do_arguments":        r("echo then else do\n", 6000),
		"let_lists":                r("let x && echo if\n", 6000),
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := syntax.NewParser().Parse(strings.NewReader(script), ""); err != nil {
				t.Fatalf("test script is not valid shell: %v", err)
			}
			if err := checkScriptNestingDepth(script); err != nil {
				t.Fatalf("lexical guard falsely rejected a valid script: %v", err)
			}
		})
	}
}

// TestCheckScriptNestingDepthQuotingContexts pins the lexical guard's handling
// of the contexts in which a bracket cannot open a nested construct.
func TestCheckScriptNestingDepthQuotingContexts(t *testing.T) {
	inert := []string{
		"echo '" + strings.Repeat("(", MaxNestingDepth+100) + "'", // single-quoted literal
		"echo " + strings.Repeat("\\(", MaxNestingDepth+100),      // backslash-escaped
		"# " + strings.Repeat("(", MaxNestingDepth+100),           // whole-line comment
		"echo ok # " + strings.Repeat("(", MaxNestingDepth+100),   // trailing comment
	}
	for i, s := range inert {
		if err := checkScriptNestingDepth(s); err != nil {
			t.Errorf("case %d: inert brackets must not count toward depth: %v", i, err)
		}
	}
	if err := checkScriptNestingDepth(strings.Repeat("( ", MaxNestingDepth+1)); err == nil {
		t.Errorf("real bracket nesting past the limit must be rejected")
	}
}

// --- subprocess crash test -------------------------------------------------

const deepNestingChildEnv = "RSHELL_DEEP_NESTING_CHILD"

// TestDeepNestingDoesNotCrashProcess reproduces the real exploit depth — far
// past the stack-overflow threshold on unpatched code — in a re-exec child, so
// that an uncatchable "fatal error: stack overflow" is observed as a failed
// child process rather than aborting this test binary. On main the child dies
// with a runtime-fatal stack overflow; with the fix it returns a normal error
// and exits 0.
func TestDeepNestingDoesNotCrashProcess(t *testing.T) {
	if os.Getenv(deepNestingChildEnv) == "1" {
		deepNestingChild()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestDeepNestingDoesNotCrashProcess", "-test.v")
	cmd.Env = append(os.Environ(), deepNestingChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if bytes.Contains(out, []byte("stack overflow")) {
		t.Fatalf("deeply nested script crashed the process with a stack overflow:\n%s", out)
	}
	if err != nil {
		t.Fatalf("deeply nested script crashed the child process: %v\n%s", err, out)
	}
}

// deepNestingChild runs the documented ingestion path (ParseScript then Run) on
// genuinely crash-inducing inputs and exits 0 only if every one was handled
// without a fatal runtime throw.
func deepNestingChild() {
	const depth = 300000 // well past the stack-overflow threshold on unpatched code
	for name, script := range deepNestingScripts(depth) {
		prog, err := ParseScript(script, "")
		if err != nil {
			continue // rejected by the lexical guard before parsing — fine
		}
		var stdout, stderr bytes.Buffer
		runner, nerr := New(StdIO(nil, &stdout, &stderr))
		if nerr != nil {
			fmt.Printf("child: New failed for %s: %v\n", name, nerr)
			os.Exit(3)
		}
		// Must return an error, not crash.
		_ = runner.Run(context.Background(), prog)
	}
	fmt.Println("child: all deep scripts handled without crashing")
	os.Exit(0)
}

const lexicalBypassChildEnv = "RSHELL_LEXICAL_BYPASS_CHILD"

// TestLexicalBypassDoesNotCrashParser mirrors the review's reproduction: in a
// re-exec child with the goroutine stack capped at 8 MiB, ParseScript is fed
// each bypass shape at a depth that overflows the parser at that cap. Before
// the fix the guard accepted them and the child died with "fatal error: stack
// overflow" inside the parser; now each is rejected before parsing.
func TestLexicalBypassDoesNotCrashParser(t *testing.T) {
	if os.Getenv(lexicalBypassChildEnv) == "1" {
		debug.SetMaxStack(8 << 20)
		for name, script := range lexicalBypassScripts(60000) {
			if _, err := ParseScript(script, ""); err == nil {
				fmt.Printf("child: %s was accepted\n", name)
				os.Exit(3)
			}
		}
		fmt.Println("child: all bypass shapes rejected")
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestLexicalBypassDoesNotCrashParser", "-test.v")
	cmd.Env = append(os.Environ(), lexicalBypassChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if bytes.Contains(out, []byte("stack overflow")) {
		t.Fatalf("a bypass shape crashed the parser with a stack overflow:\n%s", out)
	}
	if err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
}
