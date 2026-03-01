// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

// allowedCommands maps each permitted command name to its allowed flags.
// Only commands in this map may appear as the first literal word of a simple command.
// An empty slice means no flags are permitted for that command.
var allowedCommands = map[string]map[string]bool{
	// ls - List directory contents
	"ls": toSet(
		"-l", // long listing format
		"-a", // include entries starting with .
		"-h", // human-readable sizes
		"-t", // sort by modification time
		"-R", // list subdirectories recursively
		"-r", // reverse order while sorting
		"-1", // one entry per line
		"-d", // list directories themselves, not contents
	),
	// tail - Output the last part of files
	"tail": toSet(
		"-n",      // output the last N lines
		"-c",      // output the last N bytes
		"--lines", // output the last N lines (long form)
		"--bytes", // output the last N bytes (long form)
	),
	// head - Output the first part of files
	"head": toSet(
		"-n",      // output the first N lines
		"-c",      // output the first N bytes
		"--lines", // output the first N lines (long form)
		"--bytes", // output the first N bytes (long form)
	),
	// find - Search for files in a directory hierarchy
	"find": toSet(
		"-name",     // match filename pattern
		"-iname",    // case-insensitive filename match
		"-type",     // match file type (f=file, d=dir, l=link)
		"-maxdepth", // limit search depth
		"-mindepth", // minimum search depth
		"-size",     // match by file size
		"-mtime",    // match by modification time (days)
		"-mmin",     // match by modification time (minutes)
		"-print",    // print pathname
		"-path",     // match full path pattern
		"-not",      // negate expression
		"-empty",    // match empty files/directories
		"-newer",    // match files newer than reference
	),
	// grep - Search for patterns in files
	"grep": toSet(
		"-i",            // case-insensitive matching
		"-v",            // invert match (select non-matching lines)
		"-c",            // count matching lines
		"-l",            // list files with matches
		"-n",            // show line numbers
		"-r",            // recursive search
		"-e",            // specify pattern
		"-w",            // match whole words only
		"-E",            // extended regular expressions
		"-F",            // fixed string matching
		"-m",            // stop after N matches
		"-A",            // print N lines after match
		"-B",            // print N lines before match
		"-C",            // print N lines of context
		"--include",     // search only matching files
		"--exclude",     // skip matching files
		"--exclude-dir", // skip matching directories
	),
	// wc - Print newline, word, and byte counts
	"wc": toSet(
		"-l", // print line count
		"-w", // print word count
		"-c", // print byte count
	),
	// sort - Sort lines of text files
	"sort": toSet(
		"-r", // reverse sort order
		"-n", // numeric sort
		"-u", // unique (remove duplicates)
		"-k", // sort by key
		"-t", // field separator
		"-f", // ignore case
		"-h", // human-numeric sort
	),
	// uniq - Report or omit repeated lines
	"uniq": toSet(
		"-c", // prefix lines by occurrence count
		"-d", // only print duplicate lines
		"-i", // ignore case when comparing
	),
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
