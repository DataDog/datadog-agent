// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

// allowedCommands maps each permitted command name to its allowed flags.
// Only commands in this map may appear as the first literal word of a simple command.
// An empty slice means no flags are permitted for that command.
var allowedCommands = map[string]map[string]bool{
	// -- Standard POSIX / GNU utilities --
	"echo": toSet("-n", "-e", "-E"),
	"pwd":  toSet("-L", "-P"),
	"cd":   toSet("-L", "-P"),
	"ls": toSet(
		"-l", "-a", "-A", "-R", "-r", "-t", "-S", "-h", "-d", "-1",
		"-F", "-p", "-i", "-s", "-n", "-g", "-o", "-G", "-T", "-U",
		"-c", "-u",
	),
	"tail": toSet("-n", "-c", "--lines", "--bytes"),
	"head": toSet("-n", "-c", "--lines", "--bytes"),
	"find": toSet(
		"-name", "-iname", "-type", "-maxdepth", "-mindepth", "-empty",
		"-size", "-newer", "-print", "-print0", "-path", "-ipath",
		"-regex", "-iregex", "-not", "-and", "-or", "-perm", "-user",
		"-group", "-mtime", "-atime", "-ctime", "-mmin", "-amin",
		"-cmin", "-depth", "-prune", "-true", "-false", "-readable",
		"-writable", "-executable", "-links", "-inum", "-samefile", "-xtype",
	),
	"cat": toSet(
		"-n", "-b", "-s", "-v", "-e", "-t", "-E", "-T", "-A",
		"--number", "--number-nonblank", "--squeeze-blank",
		"--show-ends", "--show-tabs", "--show-all", "--show-nonprinting",
	),
	"grep": toSet(
		"-i", "-v", "-c", "-l", "-L", "-n", "-H", "-h", "-r", "-R",
		"-e", "-w", "-x", "-q", "-s", "-m", "-E", "-F", "-P",
		"-o", "-a", "-b", "-d", "-I",
		"--include", "--exclude", "--exclude-dir", "--include-dir",
		"-A", "-B", "-C", "--color", "--colour", "-Z", "--null",
		// NOTE: -f intentionally excluded — reads patterns from files, enabling arbitrary file read
	),
	"wc": toSet(
		"-c", "-m", "-l", "-w", "-L",
		"--bytes", "--chars", "--lines", "--words", "--max-line-length",
	),
	"sort": toSet(
		"-r", "-n", "-f", "-u", "-b", "-t", "-k", "-s", "-g", "-h",
		"-M", "-V", "-d", "-i", "-R", "-z",
		"--stable", "--reverse", "--numeric-sort", "--unique",
		"--ignore-leading-blanks",
	),
	"uniq": toSet(
		"-c", "-d", "-u", "-f", "-s", "-i", "-w",
		"--count", "--repeated", "--unique", "--skip-fields",
		"--skip-chars", "--ignore-case", "--check-chars",
	),
	"sed": toSet(
		"-n", "-e", "-E", "-r",
		"--quiet", "--silent", "--regexp-extended", "--posix",
	),
	"true":     toSet(),
	"false":    toSet(),
	"break":    toSet(),
	"continue": toSet(),

	// -- Shell builtins needed for allowed control flow features --
	":":        toSet(),
	"exit":     toSet(),
	"set":      toSet("-e", "-u", "-x", "-o", "-f", "-n", "-v", "-h", "-b", "-C", "+e", "+u", "+x", "+o", "+f", "+n", "+v", "+h", "+b", "+C"),
	"shift":    toSet(),
	"unset":    toSet(),
	"return":   toSet(),
	"read":     toSet("-r", "-p", "-n", "-t", "-d", "-a", "-s"),
	"declare":  toSet("-a", "-A", "-f", "-i", "-l", "-r", "-t", "-u", "-x", "-g", "-p"),
	// NOTE: -n intentionally excluded from declare/local — creates namerefs (indirect variable manipulation)
	"local":    toSet("-a", "-A", "-i", "-l", "-r", "-u", "-x"),
	"export":   toSet("-n", "-p"),
	// NOTE: -f intentionally excluded from export — exports functions, ShellShock-related risk
	"readonly": toSet("-a", "-A", "-f", "-p"),
	"test":     toSet(),
	"[":        toSet(),
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
