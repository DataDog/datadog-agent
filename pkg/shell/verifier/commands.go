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
	"echo":     "Print text to stdout",
	"pwd":      "Print working directory",
	"cd":       "Change directory",
	"ls":       "List directory contents",
	"tail":     "Output the last part of files",
	"head":     "Output the first part of files",
	"find":     "Search for files in a directory hierarchy",
	"cat":      "Concatenate and print files",
	"grep":     "Search for patterns in files",
	"wc":       "Print newline, word, and byte counts",
	"sort":     "Sort lines of text files",
	"uniq":     "Report or omit repeated lines",
	"sed":      "Stream editor for filtering and transforming text",
	"true":     "Do nothing, successfully",
	"false":    "Do nothing, return failure",
	"break":    "Exit from a loop",
	"continue": "Continue to next loop iteration",
	":":        "Null command (no-op)",
	"exit":     "Exit the shell",
	"test":     "Evaluate conditional expression",
	"[":        "Evaluate conditional expression (bracket form)",
}

// flagDescriptions provides human-readable descriptions for each allowed flag.
var flagDescriptions = map[string]map[string]string{
	"echo": {
		"-n": "do not output trailing newline",
		"-e": "enable backslash escapes",
		"-E": "disable backslash escapes",
	},
	"pwd": {
		"-L": "use PWD from environment (logical)",
		"-P": "avoid all symlinks (physical)",
	},
	"cd": {
		"-L": "follow symlinks (logical)",
		"-P": "use physical directory structure",
	},
	"ls": {
		"-l": "long listing format",
		"-a": "include entries starting with .",
		"-A": "like -a but omit . and ..",
		"-R": "list subdirectories recursively",
		"-r": "reverse order while sorting",
		"-t": "sort by modification time",
		"-S": "sort by file size",
		"-h": "human-readable sizes",
		"-d": "list directories themselves, not contents",
		"-1": "one entry per line",
		"-F": "append indicator (*/=>@|)",
		"-p": "append / to directories",
		"-i": "print inode number",
		"-s": "print allocated size",
		"-n": "numeric uid/gid",
		"-g": "like -l but omit owner",
		"-o": "like -l but omit group",
		"-G": "inhibit display of group",
		"-T": "display complete time",
		"-U": "do not sort; list in directory order",
		"-c": "sort by ctime",
		"-u": "sort by access time",
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
		"-name":       "match filename pattern",
		"-iname":      "case-insensitive filename match",
		"-type":       "match file type (f=file, d=dir, l=link)",
		"-maxdepth":   "limit search depth",
		"-mindepth":   "minimum search depth",
		"-empty":      "match empty files/directories",
		"-size":       "match by file size",
		"-newer":      "match files newer than reference",
		"-print":      "print pathname",
		"-print0":     "print null-terminated pathname",
		"-path":       "match full path pattern",
		"-ipath":      "case-insensitive path match",
		"-regex":      "match path with regex",
		"-iregex":     "case-insensitive regex",
		"-not":        "negate expression",
		"-and":        "combine expressions with AND",
		"-or":         "combine expressions with OR",
		"-perm":       "match file permissions",
		"-user":       "match file owner",
		"-group":      "match file group",
		"-mtime":      "match by modification time (days)",
		"-atime":      "match by access time (days)",
		"-ctime":      "match by change time (days)",
		"-mmin":       "match by modification time (minutes)",
		"-amin":       "match by access time (minutes)",
		"-cmin":       "match by change time (minutes)",
		"-depth":      "process directory contents before directory",
		"-prune":      "do not descend into directory",
		"-true":       "always true",
		"-false":      "always false",
		"-readable":   "match readable files",
		"-writable":   "match writable files",
		"-executable": "match executable files",
		"-links":      "match by number of hard links",
		"-inum":       "match by inode number",
		"-samefile":   "match files with same inode",
		"-xtype":      "match type after symlink resolution",
	},
	"cat": {
		"-n":                   "number all output lines",
		"-b":                   "number nonblank output lines",
		"-s":                   "squeeze consecutive blank lines",
		"-v":                   "show nonprinting characters",
		"-e":                   "equivalent to -vE",
		"-t":                   "equivalent to -vT",
		"-E":                   "display $ at end of each line",
		"-T":                   "display TAB as ^I",
		"-A":                   "equivalent to -vET",
		"--number":             "number all output lines (long form)",
		"--number-nonblank":    "number nonblank lines (long form)",
		"--squeeze-blank":      "squeeze blank lines (long form)",
		"--show-ends":          "show $ at end of lines (long form)",
		"--show-tabs":          "show TAB as ^I (long form)",
		"--show-all":           "show all (long form)",
		"--show-nonprinting":   "show nonprinting chars (long form)",
	},
	"grep": {
		"-i":            "case-insensitive matching",
		"-v":            "invert match (select non-matching lines)",
		"-c":            "count matching lines",
		"-l":            "list files with matches",
		"-L":            "list files without matches",
		"-n":            "show line numbers",
		"-H":            "print filename with matches",
		"-h":            "suppress filename prefix",
		"-r":            "recursive search",
		"-R":            "recursive search (follow symlinks)",
		"-e":            "specify pattern",
		"-w":            "match whole words only",
		"-x":            "match whole lines only",
		"-q":            "quiet mode (exit status only)",
		"-s":            "suppress error messages",
		"-m":            "stop after N matches",
		"-E":            "extended regular expressions",
		"-F":            "fixed string matching",
		"-P":            "Perl-compatible regex",
		"-o":            "print only matched parts",
		"-a":            "treat binary as text",
		"-b":            "print byte offset",
		"-d":            "action for directories",
		"-I":            "treat binary as non-matching",
		"--include":     "search only matching files",
		"--exclude":     "skip matching files",
		"--exclude-dir": "skip matching directories",
		"--include-dir": "search only matching directories",
		"-A":            "print N lines after match",
		"-B":            "print N lines before match",
		"-C":            "print N lines of context",
		"--color":       "highlight matches",
		"--colour":      "highlight matches (British spelling)",
		"-Z":            "print null byte after filename",
		"--null":        "print null byte after filename (long form)",
	},
	"wc": {
		"-c":               "print byte count",
		"-m":               "print character count",
		"-l":               "print line count",
		"-w":               "print word count",
		"-L":               "print maximum line length",
		"--bytes":          "print byte count (long form)",
		"--chars":          "print character count (long form)",
		"--lines":          "print line count (long form)",
		"--words":          "print word count (long form)",
		"--max-line-length": "print max line length (long form)",
	},
	"sort": {
		"-r":                       "reverse sort order",
		"-n":                       "numeric sort",
		"-f":                       "ignore case",
		"-u":                       "unique (remove duplicates)",
		"-b":                       "ignore leading blanks",
		"-t":                       "field separator",
		"-k":                       "sort by key",
		"-s":                       "stable sort",
		"-g":                       "general numeric sort",
		"-h":                       "human-numeric sort",
		"-M":                       "month sort",
		"-V":                       "version sort",
		"-d":                       "dictionary order",
		"-i":                       "ignore nonprinting characters",
		"-R":                       "random sort",
		"-z":                       "null-terminated lines",
		"--stable":                 "stable sort (long form)",
		"--reverse":                "reverse sort (long form)",
		"--numeric-sort":           "numeric sort (long form)",
		"--unique":                 "unique (long form)",
		"--ignore-leading-blanks":  "ignore leading blanks (long form)",
	},
	"uniq": {
		"-c":             "prefix lines by occurrence count",
		"-d":             "only print duplicate lines",
		"-u":             "only print unique lines",
		"-f":             "skip N fields before comparing",
		"-s":             "skip N chars before comparing",
		"-i":             "ignore case when comparing",
		"-w":             "compare no more than N chars",
		"--count":        "prefix by count (long form)",
		"--repeated":     "show duplicates (long form)",
		"--unique":       "show unique (long form)",
		"--skip-fields":  "skip fields (long form)",
		"--skip-chars":   "skip chars (long form)",
		"--ignore-case":  "ignore case (long form)",
		"--check-chars":  "compare N chars (long form)",
	},
	"sed": {
		"-n":                 "suppress automatic printing",
		"-e":                 "add script expression",
		"-E":                 "use extended regular expressions",
		"-r":                 "use extended regular expressions (alias)",
		"--quiet":            "suppress printing (long form)",
		"--silent":           "suppress printing (long form)",
		"--regexp-extended":  "extended regex (long form)",
		"--posix":            "disable GNU extensions",
	},
}

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

	// -- Shell builtins handled by the interpreter --
	":":    toSet(),
	"exit": toSet(),

	// -- External binaries that also exist as builtins on some systems --
	"test": toSet(),
	"[":    toSet(),
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
