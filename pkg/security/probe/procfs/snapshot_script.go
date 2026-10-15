// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"path"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// maxCommLen is the length the kernel cuts the name of a process to.
const maxCommLen = 15

// ScriptPath returns the path of the script that a process runs through an
// interpreter, or "" when it runs none it can tell. comm, exe and argv are the
// name, the executable and the arguments of the process.
//
// A process named after its executable or its argv[0] runs that program by
// name. When interpreters holds the grammar of that program, its script is its
// first operand, past its options. Any other process runs the interpreter of
// the shebang of its script: the kernel names it after the script, cut to
// maxCommLen bytes, and runs the interpreter as the shebang writes it, an
// absolute path, with the optional argument of the shebang, then the script as
// the process ran it, then the arguments of the script. The first of argv[1]
// and argv[2] named like the process is then the script.
//
// Either way a script counts as an absolute path in clean form, out of /proc
// and /dev/fd, whose files stand for open file descriptors.
func ScriptPath(comm, exe string, argv []string) string {
	if len(argv) < 2 {
		return ""
	}
	// procfs appends " (deleted)" to the path of an executable replaced on
	// disk.
	exeName := path.Base(strings.TrimSuffix(exe, " (deleted)"))
	argv0Name := path.Base(argv[0])
	if namedAfter(comm, exeName) || namedAfter(comm, argv0Name) {
		return interpreterScript(exeName, argv0Name, argv[1:])
	}
	if !path.IsAbs(argv[0]) {
		return ""
	}
	for _, arg := range argv[1:min(len(argv), 3)] {
		if namedAfter(comm, path.Base(arg)) {
			return scriptArg(arg)
		}
	}
	return ""
}

// StatScript returns the file of script, the script of the process pid, with
// the metadata of the path itself, a symlink included, read through the root of
// the process. The metadata is zero for a file statx fails on.
func StatScript(pid uint32, script string) model.FileEvent {
	file := model.FileEvent{FileFields: statProcessFile(pid, script, unix.AT_SYMLINK_NOFOLLOW)}
	file.SetPathnameStr(script)
	file.SetBasenameStr(path.Base(script))
	return file
}

// namedAfter reports whether comm, the name of a process, comes from the file
// name name: name itself, or name cut to maxCommLen bytes.
func namedAfter(comm, name string) bool {
	return comm == name || len(comm) == maxCommLen && strings.HasPrefix(name, comm)
}

// scriptArg returns arg when it can name a script: an absolute path in clean
// form, out of /proc and /dev/fd. Clean form keeps the path inside the root of
// the process it is joined to.
func scriptArg(arg string) string {
	if !path.IsAbs(arg) || path.Clean(arg) != arg || strings.HasPrefix(arg, "/proc/") || strings.HasPrefix(arg, "/dev/fd/") {
		return ""
	}
	return arg
}

// interpreterScript returns the script of the program named exeName, run as
// argv0Name with the arguments args, or "" for a program outside interpreters.
// Version digits end the names of interpreters, as in python3.10, and busybox
// runs the applet argv0Name names.
func interpreterScript(exeName, argv0Name string, args []string) string {
	name := strings.TrimRight(exeName, "0123456789.")
	if name == "busybox" {
		name = strings.TrimRight(argv0Name, "0123456789.")
	}
	g, ok := interpreters[name]
	if !ok {
		return ""
	}
	return scriptArg(g.operand(args))
}

// optionKind tells how an option of an interpreter takes its value. An option
// missing from a grammar has the zero kind and takes no value.
type optionKind uint8

const (
	// rest takes the rest of its argument as its value, which may be empty.
	rest optionKind = iota + 1
	// restOrNext takes the rest of its argument as its value, or the next
	// argument when it ends its argument.
	restOrNext
	// next takes the next argument as its value, and the letters after it in
	// its argument remain options.
	next
	// octal takes the octal digits that follow it as its value.
	octal
	// inline runs code from the command line, a module or stdin, in place of a
	// script.
	inline
)

// grammar describes the command line of an interpreter: options, then
// operands, the first of which is the script.
type grammar struct {
	// options maps the options that take a value or run inline code to their
	// kind: a letter for an option in a cluster, the argument up to its = for
	// any other.
	options map[string]optionKind
	// clusters reports whether letter options cluster after a dash, as in -uB.
	clusters bool
	// shell reports whether + starts a cluster too, as in +o, and whether a
	// lone - ends the options as -- does. A lone - reads the program from stdin
	// otherwise.
	shell bool
}

// operand returns the first operand in args, the arguments of an interpreter
// after its name, or "" when the interpreter runs inline code or stdin.
func (g *grammar) operand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--", arg == "-" && g.shell:
			return at(args, i+1)
		case arg == "-":
			return ""
		case strings.HasPrefix(arg, "--"), !g.clusters && strings.HasPrefix(arg, "-"):
			name, _, attached := strings.Cut(arg, "=")
			switch g.options[name] {
			case inline:
				return ""
			case next, restOrNext:
				if !attached {
					i++
				}
			}
		case strings.HasPrefix(arg, "-"), g.shell && strings.HasPrefix(arg, "+"):
			skip, ok := g.cluster(arg[1:])
			if !ok {
				return ""
			}
			i += skip
		default:
			return arg
		}
	}
	return ""
}

// cluster returns how many of the next arguments the options in letters, a
// cluster of letter options, take as values. It reports false when one of them
// runs inline code.
func (g *grammar) cluster(letters string) (int, bool) {
	skip := 0
	for j := 0; j < len(letters); j++ {
		switch g.options[letters[j:j+1]] {
		case inline:
			return 0, false
		case rest:
			return skip, true
		case restOrNext:
			if j == len(letters)-1 {
				skip++
			}
			return skip, true
		case next:
			skip++
		case octal:
			for j+1 < len(letters) && '0' <= letters[j+1] && letters[j+1] <= '7' {
				j++
			}
		}
	}
	return skip, true
}

// at returns args[i], or "" past the end of args.
func at(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

// shellGrammar is the grammar of dash, bash, busybox sh and the other shells:
// -c runs a command and -s reads stdin, -o and -O set an option the next
// argument names, and bash reads the next argument as its startup file.
var shellGrammar = &grammar{
	options: map[string]optionKind{
		"c": inline, "s": inline, "o": next, "O": next,
		"--init-file": next, "--rcfile": next,
	},
	clusters: true,
	shell:    true,
}

// nodeGrammar is the grammar of Node.js, whose options never cluster. The
// options that take a value are those node --help lists with one.
var nodeGrammar = &grammar{
	options: map[string]optionKind{
		"-e": inline, "--eval": inline, "-p": inline, "--print": inline, "-pe": inline, "--run": inline,
		"-C": next, "--conditions": next, "-r": next, "--require": next, "--import": next,
		"--loader": next, "--experimental-loader": next, "--input-type": next,
		"--allow-fs-read": next, "--allow-fs-write": next, "--build-snapshot-config": next,
		"--cpu-prof-dir": next, "--cpu-prof-interval": next, "--cpu-prof-name": next,
		"--debug-port": next, "--diagnostic-dir": next, "--disable-proto": next,
		"--disable-warning": next, "--dns-result-order": next, "--env-file": next,
		"--env-file-if-exists": next, "--experimental-config-file": next,
		"--experimental-package-map": next, "--experimental-sea-config": next,
		"--heap-prof-dir": next, "--heap-prof-interval": next, "--heap-prof-name": next,
		"--heapsnapshot-near-heap-limit": next, "--heapsnapshot-signal": next,
		"--icu-data-dir": next, "--inspect-port": next, "--inspect-publish-uid": next,
		"--localstorage-file": next, "--max-http-header-size": next,
		"--max-old-space-size-percentage": next, "--network-family-autoselection-attempt-timeout": next,
		"--openssl-config": next, "--redirect-warnings": next, "--report-dir": next,
		"--report-directory": next, "--report-filename": next, "--report-signal": next,
		"--secure-heap": next, "--secure-heap-min": next, "--snapshot-blob": next,
		"--title": next, "--tls-cipher-list": next, "--tls-keylog": next,
		"--trace-event-categories": next, "--trace-event-file-pattern": next,
		"--trace-require-module": next, "--unhandled-rejections": next,
		"--use-largepages": next, "--v8-pool-size": next, "--watch-kill-signal": next,
		"--watch-path": next,
	},
}

// interpreters holds the grammars of the interpreters, by name without version
// digits. They follow the parsers of CPython, perl, ruby, Node.js, dash and
// bash.
var interpreters = map[string]*grammar{
	"python": {
		options: map[string]optionKind{
			"c": inline, "m": inline, "W": restOrNext, "X": restOrNext,
			"--check-hash-based-pycs": next,
		},
		clusters: true,
	},
	"perl": {
		options: map[string]optionKind{
			"e": inline, "E": inline, "I": restOrNext, "0": octal, "l": octal,
			"C": rest, "d": rest, "D": rest, "F": rest, "i": rest, "m": rest, "M": rest, "V": rest, "x": rest,
		},
		clusters: true,
	},
	"ruby": {
		options: map[string]optionKind{
			"e": inline,
			"C": restOrNext, "E": restOrNext, "I": restOrNext, "r": restOrNext, "X": restOrNext,
			"0": octal, "F": rest, "i": rest, "K": rest, "W": rest, "x": rest,
			"--backtrace-limit": next, "--disable": next, "--dump": next, "--enable": next,
			"--encoding": next, "--external-encoding": next, "--internal-encoding": next,
		},
		clusters: true,
	},
	"node":   nodeGrammar,
	"nodejs": nodeGrammar,
	"ash":    shellGrammar,
	"bash":   shellGrammar,
	"dash":   shellGrammar,
	"ksh":    shellGrammar,
	"mksh":   shellGrammar,
	"sh":     shellGrammar,
	"zsh":    shellGrammar,
}
