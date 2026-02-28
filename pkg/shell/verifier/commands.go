// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

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
