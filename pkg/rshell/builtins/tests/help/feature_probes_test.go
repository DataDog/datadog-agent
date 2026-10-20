// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package help_test

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// TestFeatureProbesMatchHelp guards against drift between the help text in
// builtins/features.go and the interpreter's real behavior. For each
// Supported/Unsupported item in the four syntactic features (variables,
// control-flow, pipes-redirections, quoting-expansion) a probe script is
// executed and its stderr is checked:
//
//   - Supported items must NOT produce a "not supported" rejection error
//     (other runtime errors such as permission-denied are tolerated — they
//     prove the feature parsed and reached execution).
//   - Unsupported items MUST produce a "not supported" rejection and exit
//     with a non-zero status.
//
// The probe map is keyed by the exact item string from featureRegistry, so
// adding or rewording an entry without updating the probe causes the
// coverage check to fail — forcing future contributors to keep help and
// implementation in lockstep.
func TestFeatureProbesMatchHelp(t *testing.T) {
	for _, name := range syntacticFeatures {
		feature, ok := builtins.Feature(name)
		if !ok {
			t.Fatalf("feature %q missing from registry", name)
		}
		probes := featureProbes[name]
		if probes == nil {
			t.Fatalf("no probe map for syntactic feature %q", name)
		}

		for _, item := range feature.Supported {
			t.Run(name+"/supported/"+shortLabel(item), func(t *testing.T) {
				script, ok := probes[item]
				if !ok {
					t.Fatalf("missing probe for supported item %q in feature %q (add one in feature_probes_test.go)", item, name)
				}
				_, stderr, _ := runScript(t, script, "", interpoption.AllowAllCommands().(interp.RunnerOption))
				if strings.Contains(stderr, "not supported") {
					t.Fatalf("supported item %q produced a 'not supported' rejection:\nscript: %s\nstderr: %s", item, script, stderr)
				}
			})
		}
		for _, item := range feature.Unsupported {
			t.Run(name+"/unsupported/"+shortLabel(item), func(t *testing.T) {
				script, ok := probes[item]
				if !ok {
					t.Fatalf("missing probe for unsupported item %q in feature %q (add one in feature_probes_test.go)", item, name)
				}
				_, stderr, code := runScript(t, script, "", interpoption.AllowAllCommands().(interp.RunnerOption))
				if code == 0 {
					t.Fatalf("unsupported item %q exited 0 — interpreter accepted a construct the help claims is unsupported:\nscript: %s\nstderr: %s", item, script, stderr)
				}
				if !strings.Contains(stderr, "not supported") {
					t.Fatalf("unsupported item %q did not produce a 'not supported' rejection:\nscript: %s\nstderr: %s", item, script, stderr)
				}
			})
		}
	}
}

// shortLabel produces a stable, short test-name fragment from a feature item.
func shortLabel(s string) string {
	if i := strings.IndexAny(s, ".:;,"); i > 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == ' ':
			return '_'
		case r == '/' || r == '\\':
			return '-'
		default:
			return r
		}
	}, s)
}

var syntacticFeatures = []string{
	"variables",
	"control-flow",
	"pipes-redirections",
	"quoting-expansion",
}

// featureProbes maps each Supported/Unsupported item (verbatim from
// featureRegistry) to a minimal script that exercises it. Adding a new
// item to featureRegistry without updating this map causes
// TestFeatureProbesMatchHelp to fail.
var featureProbes = map[string]map[string]string{
	"variables": {
		// Supported
		"VAR=value assignment and expansion with $VAR or ${VAR}.":                                                           "X=1; echo $X ${X}",
		"Inline assignment with VAR=value command, scoped to that command.":                                                 "X=1 echo done",
		"$? expands to the previous command's exit code.":                                                                   "false; echo $?",
		"Command substitution with $(cmd), legacy backquotes, and $(<file) when cat is allowed; output is capped at 1 MiB.": "echo $(echo a) `echo b`",
		// Unsupported
		"Arithmetic expansion: $(( expr )).": "echo $((1+1))",
		"Arrays and array assignments.":      "A=(a b)",
		"Append assignment with VAR+=value.": "X=a; X+=b",
		"Advanced parameter expansion operations such as ${#var}, defaults, slicing, pattern replacement, indirect expansion, and case conversion.": "X=a; echo ${#X}",
		"Positional parameters ($1, $@, $#, $0) and special variables such as $! and $LINENO.":                                                      "echo $1",
	},
	"control-flow": {
		// Supported
		"for VAR in WORDS; do CMDS; done.": "for x in a; do :; done",
		"while CONDITION; do CMDS; done — runs CMDS while the condition's last command exits 0.":        "while false; do :; done",
		"until CONDITION; do CMDS; done — runs CMDS while the condition's last command exits non-zero.": "until true; do :; done",
		"if / elif / else conditionals.":                                                         "if true; then :; elif false; then :; else :; fi",
		"AND/OR lists with && and ||, plus ! exit-code negation.":                                "true && ! false || true",
		"Brace groups with { CMDS; } and command separators with ; or newline.":                  "{ :; }",
		"Subshells with ( CMDS ); variable changes and exit are isolated from the parent shell.": "(:)",
		// Unsupported
		"case and select statements.":       "case x in x) :;; esac",
		"C-style for loops: for (( ... )).": "for ((i=0;i<1;i++)); do :; done",
		"Shell functions: name() { ... }.":  "f(){ :; }",
	},
	"pipes-redirections": {
		// Supported
		"Pipelines with | pipe stdout from one command to the next.":                                                 "echo x | cat",
		"Input redirection with < reads files through AllowedPaths.":                                                 ": </dev/null",
		"Heredocs with <<DELIM and <<-DELIM.":                                                                        "cat <<EOT\nx\nEOT\n",
		"Output redirection to /dev/null only: >/dev/null, 2>/dev/null, &>/dev/null, >>/dev/null, and &>>/dev/null.": "echo x >/dev/null",
		"File descriptor duplication between stdout and stderr with 2>&1 and >&2.":                                   "echo x 2>&1",
		// Unsupported
		"Writing, appending, or redirecting output to any file other than /dev/null.": "echo x >/tmp/rshell-probe-should-not-write",
		"Pipe stdout and stderr together with |&.":                                    "echo x |& cat",
		"Herestrings with <<<.":                                                       "cat <<<x",
		"Read-write redirection with <> and input fd duplication with <&N.":           "cat <&0",
	},
	"quoting-expansion": {
		// Supported
		"Single quotes preserve literal text.": "echo 'x'",
		"Double quotes preserve words while still allowing supported variable and command substitution.": "echo \"x\"",
		"Globbing with *, ?, character classes like [abc] and ranges like [a-z] or [!a].":                "echo [abc]",
		"Line continuation with a trailing backslash and comments beginning with #.":                     "echo a # comment\n",
		// Unsupported
		"Extended globbing such as @(pat) and *(pat).": "echo @(a|b)",
		"Tilde expansion: ~, ~/path, and ~user.":       "echo ~",
		"Process substitution: <(cmd) and >(cmd).":     "cat <(echo a)",
	},
}
