// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package builtins

// FeatureMeta holds metadata for an rshell language/runtime feature exposed by
// the help builtin. The list is maintained in Go code so `help` output stays
// deterministic and can be validated against builtin command names.
type FeatureMeta struct {
	Name        string
	Description string
	Supported   []string
	Unsupported []string
	Notes       []string
}

// Keep featureRegistry aligned with the feature categories documented in
// SHELL_FEATURES.md.
var featureRegistry = []FeatureMeta{
	{
		Name:        "commands",
		Description: "Registered commands run inside the interpreter; no unregistered commands without an external handler.",
		Supported: []string{
			"Registered rshell commands run inside the interpreter and under the active AllowedCommands policy.",
			"Commands perform filesystem access through the AllowedPaths sandbox unless explicitly documented otherwise.",
			"Use `help` to list enabled commands and `help <command>` or `<command> --help` for command-specific details.",
		},
		Unsupported: []string{
			"All other commands return exit code 127 unless an external command handler is configured and allows them.",
			"Command options that would write files or otherwise bypass sandbox safety are rejected. External program execution is gated by AllowedCommands (e.g. `find -exec`/`-execdir` runs only commands the policy allows).",
		},
	},
	{
		Name:        "variables",
		Description: "Assignments, expansion, inline env, command substitution; no arrays/arithmetic/advanced params.",
		Supported: []string{
			"VAR=value assignment and expansion with $VAR or ${VAR}.",
			"Inline assignment with VAR=value command, scoped to that command.",
			"$? expands to the previous command's exit code.",
			"Command substitution with $(cmd), legacy backquotes, and $(<file) when cat is allowed; output is capped at 1 MiB.",
		},
		Unsupported: []string{
			"Arithmetic expansion: $(( expr )).",
			"Arrays and array assignments.",
			"Append assignment with VAR+=value.",
			"Advanced parameter expansion operations such as ${#var}, defaults, slicing, pattern replacement, indirect expansion, and case conversion.",
			"Positional parameters ($1, $@, $#, $0) and special variables such as $! and $LINENO.",
		},
	},
	{
		Name:        "control-flow",
		Description: "for/while/until/if/&&/||/!/groups/subshells; no case/select/functions.",
		Supported: []string{
			"for VAR in WORDS; do CMDS; done.",
			"while CONDITION; do CMDS; done — runs CMDS while the condition's last command exits 0.",
			"until CONDITION; do CMDS; done — runs CMDS while the condition's last command exits non-zero.",
			"if / elif / else conditionals.",
			"AND/OR lists with && and ||, plus ! exit-code negation.",
			"Brace groups with { CMDS; } and command separators with ; or newline.",
			"Subshells with ( CMDS ); variable changes and exit are isolated from the parent shell.",
		},
		Unsupported: []string{
			"case and select statements.",
			"C-style for loops: for (( ... )).",
			"Shell functions: name() { ... }.",
		},
	},
	{
		Name:        "pipes-redirections",
		Description: "Pipes, stdin/heredocs, /dev/null redirects, fd dup; no arbitrary file writes.",
		Supported: []string{
			"Pipelines with | pipe stdout from one command to the next.",
			"Input redirection with < reads files through AllowedPaths.",
			"Heredocs with <<DELIM and <<-DELIM.",
			"Output redirection to /dev/null only: >/dev/null, 2>/dev/null, &>/dev/null, >>/dev/null, and &>>/dev/null.",
			"File descriptor duplication between stdout and stderr with 2>&1 and >&2.",
		},
		Unsupported: []string{
			"Writing, appending, or redirecting output to any file other than /dev/null.",
			"Pipe stdout and stderr together with |&.",
			"Herestrings with <<<.",
			"Read-write redirection with <> and input fd duplication with <&N.",
		},
	},
	{
		Name:        "quoting-expansion",
		Description: "Quotes, globbing, continuations, comments; no extglob/tilde/process substitution.",
		Supported: []string{
			"Single quotes preserve literal text.",
			"Double quotes preserve words while still allowing supported variable and command substitution.",
			"Globbing with *, ?, character classes like [abc] and ranges like [a-z] or [!a].",
			"Line continuation with a trailing backslash and comments beginning with #.",
		},
		Unsupported: []string{
			"Extended globbing such as @(pat) and *(pat).",
			"Tilde expansion: ~, ~/path, and ~user.",
			"Process substitution: <(cmd) and >(cmd).",
		},
	},
	{
		Name:        "execution",
		Description: "AllowedCommands, AllowedPaths, timeouts, ProcPath; no background jobs/coprocs/time.",
		Supported: []string{
			"AllowedCommands restricts executable commands; rshell commands use the rshell: namespace prefix.",
			"AllowedPaths restricts filesystem access to configured directories. In remediation mode, symlinked write targets are rejected with `symlinks are not supported as write targets`, and, on Unix, hard-linked write targets are rejected with `hard links are not supported as write targets` (not enforced on Windows, where no link count is available from an open handle).",
			"Whole-run timeouts can be set with context.Context, interp.MaxExecutionTime, or the CLI --timeout flag.",
			"ProcPath overrides the proc filesystem used by ps and pmap on Linux; both report process names only and do not read argv.",
			"ps supports repeatable -o/--format with the safe canonical fields pid, ppid, uid, state, tty, stime, time, comm, rss, vsz, pmem, pcpu, and etime; %cpu and %mem alias pcpu and pmem for output and sorting; --sort supports multiple fields with optional + or - direction prefixes.",
			"ps reports pcpu as a lifetime average rather than an interval measurement, renders unavailable requested metrics as -, and never reads process argv or environment data.",
		},
		Unsupported: []string{
			"External commands are blocked by default unless an external command handler is configured and the target is allowed.",
			"Background execution with cmd &, coprocesses, and the time reserved word.",
			"Extended tests with [[ ... ]] and arithmetic commands with (( ... )).",
			"Shell-defining commands such as declare, export, local, readonly, and let.",
		},
		Notes: []string{
			"On macOS, ps start and elapsed times remain available, but RSS, VSZ, PMEM, CPU time, and PCPU may be unavailable for processes the caller cannot inspect.",
			"On Windows, ps RSS and VSZ use the compatibility-sensitive SystemProcessInformation snapshot API and may be unavailable if it fails; PMEM is then unavailable too.",
		},
	},
	{
		Name:        "environment",
		Description: "Empty by default, caller Env, IFS/ALLOWED_PATHS; no host env inheritance/export.",
		Supported: []string{
			"No parent environment variables are inherited by default.",
			"Callers can provide variables with the Env option.",
			"IFS defaults to space, tab, and newline.",
			"ALLOWED_PATHS is set when AllowedPaths is configured, using the platform list separator.",
		},
		Unsupported: []string{
			"Automatic inheritance from the host process environment.",
			"export and readonly are blocked.",
		},
	},
	{
		Name:        "bash-divergences",
		Description: "Intentional bash differences captured here. Currently: one shared time reference per Run().",
		Notes: []string{
			"rshell captures time.Now() once at the start of each Run() call and shares it across commands that need a reference time, such as find -mmin/-mtime and ls -l.",
			"Bash evaluates each command against its own invocation time. The difference matters only for long-running scripts where time-sensitive predicates are evaluated much later than Run() started.",
		},
	},
}

var unsupportedSummary = []string{
	"Expansions: arithmetic $((...)), arrays, advanced ${...} operations, tilde expansion, process substitution, extended globbing.",
	"Control flow: case, select, C-style for ((...)), and shell functions.",
	"Execution: external commands by default, background jobs, coprocesses, time, [[...]], ((...)), declare/export/local/readonly/let.",
	"I/O and environment: arbitrary output file redirects, |&, herestrings, read-write redirects, input fd duplication, host env inheritance.",
}

var featureByName = buildFeatureIndex(featureRegistry)

func buildFeatureIndex(features []FeatureMeta) map[string]FeatureMeta {
	index := make(map[string]FeatureMeta, len(features))
	for _, feature := range features {
		if feature.Name == "" {
			panic("rshell feature with empty name")
		}
		if _, exists := index[feature.Name]; exists {
			panic("duplicate rshell feature: " + feature.Name)
		}
		index[feature.Name] = feature
	}
	return index
}

// Features returns rshell features in display order. The returned slice and
// each FeatureMeta's Supported/Unsupported/Notes slices are independent copies
// — callers may freely mutate them without affecting the registry.
func Features() []FeatureMeta {
	features := make([]FeatureMeta, len(featureRegistry))
	for i, f := range featureRegistry {
		features[i] = FeatureMeta{
			Name:        f.Name,
			Description: f.Description,
			Supported:   append([]string(nil), f.Supported...),
			Unsupported: append([]string(nil), f.Unsupported...),
			Notes:       append([]string(nil), f.Notes...),
		}
	}
	return features
}

// Feature returns the metadata for a named rshell feature. The returned
// FeatureMeta's Supported/Unsupported/Notes slices are independent copies
// — callers may freely mutate them without affecting the registry.
func Feature(name string) (FeatureMeta, bool) {
	feature, ok := featureByName[name]
	if !ok {
		return FeatureMeta{}, false
	}
	return FeatureMeta{
		Name:        feature.Name,
		Description: feature.Description,
		Supported:   append([]string(nil), feature.Supported...),
		Unsupported: append([]string(nil), feature.Unsupported...),
		Notes:       append([]string(nil), feature.Notes...),
	}, true
}

// UnsupportedSummary returns a concise list of intentionally unsupported
// rshell functionality for display in the top-level help output.
func UnsupportedSummary() []string {
	items := make([]string, len(unsupportedSummary))
	copy(items, unsupportedSummary)
	return items
}
