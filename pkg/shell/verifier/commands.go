// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

import "sort"

// CommandInfo describes an allowed command and its permitted flags.
type CommandInfo struct {
	Description string            `json:"description"`
	Flags       map[string]string `json:"flags"` // flag → brief description
}

// commandDescriptions provides human-readable descriptions for each allowed command.
var commandDescriptions = map[string]string{
	"ls":   "List directory contents",
	"tail": "Output the last part of files",
	"head": "Output the first part of files",
	"find": "Search for files in a directory hierarchy",
	"grep": "Search for patterns in files",
	"wc":   "Print newline, word, and byte counts",
	"sort": "Sort lines of text files",
	"uniq": "Report or omit repeated lines",
}

// flagDescriptions provides human-readable descriptions for each allowed flag.
var flagDescriptions = map[string]map[string]string{
	"ls": {
		"-l": "long listing format",
		"-a": "include entries starting with .",
		"-h": "human-readable sizes",
		"-t": "sort by modification time",
		"-R": "list subdirectories recursively",
		"-r": "reverse order while sorting",
		"-1": "one entry per line",
		"-d": "list directories themselves, not contents",
	},
	"tail": {
		"-n":      "output the last N lines",
		"-c":      "output the last N bytes",
		"--lines": "output the last N lines (long form)",
		"--bytes": "output the last N bytes (long form)",
	},
	"head": {
		"-n":      "output the first N lines",
		"-c":      "output the first N bytes",
		"--lines": "output the first N lines (long form)",
		"--bytes": "output the first N bytes (long form)",
	},
	"find": {
		"-name":     "match filename pattern",
		"-iname":    "case-insensitive filename match",
		"-type":     "match file type (f=file, d=dir, l=link)",
		"-maxdepth": "limit search depth",
		"-mindepth": "minimum search depth",
		"-size":     "match by file size",
		"-mtime":    "match by modification time (days)",
		"-mmin":     "match by modification time (minutes)",
		"-print":    "print pathname",
		"-path":     "match full path pattern",
		"-not":      "negate expression",
		"-empty":    "match empty files/directories",
		"-newer":    "match files newer than reference",
	},
	"grep": {
		"-i":            "case-insensitive matching",
		"-v":            "invert match (select non-matching lines)",
		"-c":            "count matching lines",
		"-l":            "list files with matches",
		"-n":            "show line numbers",
		"-r":            "recursive search",
		"-e":            "specify pattern",
		"-w":            "match whole words only",
		"-E":            "extended regular expressions",
		"-F":            "fixed string matching",
		"-m":            "stop after N matches",
		"-A":            "print N lines after match",
		"-B":            "print N lines before match",
		"-C":            "print N lines of context",
		"--include":     "search only matching files",
		"--exclude":     "skip matching files",
		"--exclude-dir": "skip matching directories",
	},
	"wc": {
		"-l": "print line count",
		"-w": "print word count",
		"-c": "print byte count",
	},
	"sort": {
		"-r": "reverse sort order",
		"-n": "numeric sort",
		"-u": "unique (remove duplicates)",
		"-k": "sort by key",
		"-t": "field separator",
		"-f": "ignore case",
		"-h": "human-numeric sort",
	},
	"uniq": {
		"-c": "prefix lines by occurrence count",
		"-d": "only print duplicate lines",
		"-i": "ignore case when comparing",
	},
}

// allowedCommands maps each permitted command name to its allowed flags.
// Only commands in this map may appear as the first literal word of a simple command.
// An empty slice means no flags are permitted for that command.
var allowedCommands = map[string]map[string]bool{
	"ls":   toSet("-l", "-a", "-h", "-t", "-R", "-r", "-1", "-d"),
	"tail": toSet("-n", "-c", "--lines", "--bytes"),
	"head": toSet("-n", "-c", "--lines", "--bytes"),
	"find": toSet(
		"-name", "-iname", "-type", "-maxdepth", "-mindepth",
		"-size", "-mtime", "-mmin", "-print", "-path", "-not",
		"-empty", "-newer",
	),
	"grep": toSet(
		"-i", "-v", "-c", "-l", "-n", "-r",
		"-e", "-w", "-E", "-F", "-m",
		"-A", "-B", "-C",
		"--include", "--exclude", "--exclude-dir",
	),
	"wc":   toSet("-l", "-w", "-c"),
	"sort": toSet("-r", "-n", "-u", "-k", "-t", "-f", "-h"),
	"uniq": toSet("-c", "-d", "-i"),
}

// blockedBuiltins are shell builtins that are explicitly forbidden even though
// they might technically be "commands". These can be used to escape the sandbox.
var blockedBuiltins = map[string]bool{
	"eval":   true,
	"exec":   true,
	"source": true,
	".":      true,
	"trap":   true,
}

// dangerousEnvVars are environment variables that must not be set in prefix
// assignments (e.g., PATH=/evil cmd), as they could alter command resolution
// or inject code.
var dangerousEnvVars = map[string]bool{
	"PATH":            true,
	"LD_PRELOAD":      true,
	"LD_LIBRARY_PATH": true,
	"IFS":             true,
	"CDPATH":          true,
	"ENV":             true,
	"BASH_ENV":        true,
}

// toSet converts a slice of strings to a set (map[string]bool) for O(1) lookup.
func toSet(flags ...string) map[string]bool {
	s := make(map[string]bool, len(flags))
	for _, f := range flags {
		s[f] = true
	}
	return s
}

// AllowedCommandsWithDescriptions returns the full allowlist of commands with
// their permitted flags and human-readable descriptions.
func AllowedCommandsWithDescriptions() map[string]CommandInfo {
	result := make(map[string]CommandInfo, len(allowedCommands))
	for cmd, flags := range allowedCommands {
		info := CommandInfo{
			Description: commandDescriptions[cmd],
			Flags:       make(map[string]string, len(flags)),
		}
		if descs, ok := flagDescriptions[cmd]; ok {
			for flag := range flags {
				if desc, ok := descs[flag]; ok {
					info.Flags[flag] = desc
				} else {
					info.Flags[flag] = ""
				}
			}
		} else {
			for flag := range flags {
				info.Flags[flag] = ""
			}
		}
		result[cmd] = info
	}
	return result
}

// BlockedBuiltins returns the sorted list of explicitly blocked shell builtins.
func BlockedBuiltins() []string {
	result := make([]string, 0, len(blockedBuiltins))
	for name := range blockedBuiltins {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// DangerousEnvVars returns the sorted list of environment variables that cannot
// be set in prefix assignments.
func DangerousEnvVars() []string {
	result := make([]string, 0, len(dangerousEnvVars))
	for name := range dangerousEnvVars {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// IsBlockedBuiltin returns true if the command name is an explicitly blocked builtin.
func IsBlockedBuiltin(name string) bool {
	return blockedBuiltins[name]
}

// AllowedCommandFlags returns the allowed flags for a command, and whether the
// command is in the allowlist at all.
func AllowedCommandFlags(name string) (map[string]bool, bool) {
	flags, ok := allowedCommands[name]
	return flags, ok
}
