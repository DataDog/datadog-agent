// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func checkFlag(fs *builtins.FlagSet, name string) bool {
	flag := fs.Lookup(name)
	return flag != nil && flag.Value.String() == "true"
}

// arguments models the policy-relevant operands of the fixed builtin surface.
// It deliberately does not call a handler, even with stubbed capabilities:
// several inspection builtins read the host directly. Unknown commands and
// content-driven operations fail closed to an indeterminate report.
func (c *scriptChecker) arguments(entry *CommandCheck, args []string, state *checkState) {
	flags, operands, err := builtins.InspectArgs(entry.Command, args)
	if err != nil {
		entry.issue(CheckInvalidArguments, err.Error())
		return
	}
	if entry.Command == "sed" && checkFlag(flags, "in-place") && !c.policy.remediationMode {
		entry.issue(CheckRemediationRequired, "sed: in-place editing requires remediation mode")
		return
	}
	needsWritableRoot := entry.Command == "rm" || entry.Command == "tee" || entry.Command == "truncate" ||
		(entry.Command == "sed" && checkFlag(flags, "in-place"))
	if needsWritableRoot {
		writable := false
		for _, root := range c.policy.paths.PathAccesses() {
			writable = writable || root.ReadWrite
		}
		if !writable {
			entry.issue(CheckPathNotAllowed, entry.Command+": no writable path is configured (an AllowedPaths entry with :rw is required)")
			return
		}
		if c.policy.remote {
			entry.issue(CheckRequiresExecution, "remote filesystem must confirm a writable grant root is available")
		}
	}
	if entry.Command == "logrotate" && !c.policy.pathsConfigured {
		entry.issue(CheckPathNotAllowed, "logrotate: no writable path is configured")
		return
	}
	if checkFlag(flags, "help") {
		return
	}
	paths := operands
	operation := allowedpaths.PathRead
	stdinDash := true
	switch entry.Command {
	case "echo", "printf", "true", "false", "exit", "break", "continue", "read", "tr", "uname", "help":
		return
	case "df", "free", "ip", "lsof", "pmap", "ps", "ss", "uptime", "vmstat", "ping", "ntfs-du":
		// These commands use fixed/trusted host capabilities, not user-selected
		// AllowedPaths files. OS availability and privileges are runtime checks.
		return
	case "pwd":
		return
	case "cat", "head", "tail", "cut", "wc", "strings":
	case "sort":
		for _, flag := range []string{"output", "temporary-directory", "compress-program"} {
			if flags.Changed(flag) {
				entry.issue(CheckInvalidArguments, "sort: --"+flag+" is not supported")
			}
		}
	case "uniq":
		if len(paths) > 1 {
			entry.issue(CheckInvalidArguments, "uniq: only one input operand is supported")
			return
		}
	case "ls", "du":
		operation = allowedpaths.PathLstat
		if entry.Command == "du" && checkFlag(flags, "dereference") {
			if checkFlag(flags, "no-dereference") {
				entry.issue(CheckRequiresExecution, "du symlink mode depends on ordered option evaluation")
			} else {
				operation = allowedpaths.PathRead
			}
		}
		stdinDash = false
		if len(paths) == 0 {
			paths = []string{"."}
		}
	case "stat":
		stdinDash = false
	case "grep":
		if !flags.Changed("regexp") {
			if len(paths) == 0 {
				entry.issue(CheckInvalidArguments, "grep: missing pattern")
				return
			}
			paths = paths[1:]
		}
	case "sed":
		entry.issue(CheckRequiresExecution, "embedded sed programs are not statically validated")
		if !flags.Changed("expression") {
			if len(paths) == 0 {
				entry.issue(CheckInvalidArguments, "sed: missing script")
				return
			}
			paths = paths[1:]
		}
		if checkFlag(flags, "in-place") {
			operation, stdinDash = allowedpaths.PathWrite, false
		}
	case "awk":
		entry.issue(CheckRequiresExecution, "embedded awk programs are not statically validated")
		if flags.Changed("file") {
			// awk's repeatable program-file flag has its own representation;
			// reading the program would also be needed to validate its contents.
			entry.issue(CheckRequiresExecution, "awk program-file operands require a runtime check")
			return
		}
		if len(paths) == 0 {
			entry.issue(CheckInvalidArguments, "awk: missing program")
			return
		}
		paths = paths[1:]
		for _, path := range paths {
			if strings.ContainsRune(path, '=') {
				entry.issue(CheckRequiresExecution, "awk assignment operands require a runtime check")
				return
			}
		}
	case "jq":
		// jq normalizes multi-argument options and implicit filters using
		// internal sentinel operands. Do not mistake those for filenames.
		entry.issue(CheckRequiresExecution, "jq filter and file operands require a runtime check")
		return
	case "sha256sum":
		if checkFlag(flags, "check") {
			entry.issue(CheckRequiresExecution, "checksum manifests select additional files at runtime")
		}
	case "tee", "truncate", "logrotate":
		operation, stdinDash = allowedpaths.PathWrite, false
	case "rm":
		operation, stdinDash = allowedpaths.PathRemove, false
	case "cd":
		stdinDash = false
		if len(paths) == 0 || (len(paths) == 1 && paths[0] == "-") {
			name := "HOME"
			if len(paths) == 1 {
				name = "OLDPWD"
			}
			value := state.value(name)
			if !value.known {
				entry.issue(CheckRequiresExecution, "cd target depends on runtime environment")
				return
			}
			paths = []string{value.value}
		}
		if len(paths) > 1 {
			entry.issue(CheckInvalidArguments, "cd: too many arguments")
			return
		}
		if c.policy.remote && len(c.policy.paths.PathAccesses()) == 0 {
			c.path(entry, paths[0], operation, state)
			return
		}
		// cd validates intermediate directories and -P resolves symlinks
		// before processing '..'. Ordinary file checks collapse '..' first,
		// so they cannot establish this operand's authorization in either
		// mode. Do not approve or deny it using that different resolution.
		for _, component := range strings.Split(filepath.ToSlash(paths[0]), "/") {
			if component == ".." {
				entry.issue(CheckRequiresExecution, "cd parent traversal requires intermediate directory and symlink checks on the target")
				entry.Issues[len(entry.Issues)-1].Path = paths[0]
				return
			}
		}
	case "test", "[":
		if entry.Command == "[" && len(paths) > 0 && paths[len(paths)-1] == "]" {
			paths = paths[:len(paths)-1]
		}
		if len(paths) == 2 {
			switch paths[0] {
			case "-r", "-w", "-x":
				c.path(entry, paths[1], allowedpaths.PathRead, state)
				return
			case "-L", "-h":
				c.path(entry, paths[1], allowedpaths.PathLstat, state)
				return
			case "-e", "-f", "-d", "-s", "-b", "-c", "-p", "-S", "-u", "-g", "-k", "-O", "-G", "-N":
				c.path(entry, paths[1], allowedpaths.PathStat, state)
				return
			}
		}
		entry.issue(CheckRequiresExecution, "test expression requires a runtime check")
		return
	case "xargs":
		if flags.Changed("arg-file") {
			c.path(entry, flags.Lookup("arg-file").Value.String(), allowedpaths.PathRead, state)
		}
		name := "echo"
		if len(operands) > 0 {
			name = operands[0]
		}
		if flags.Changed("replace") && strings.Contains(name, flags.Lookup("replace").Value.String()) {
			entry.issue(CheckRequiresExecution, "xargs command name depends on input")
		} else {
			child := &CommandCheck{Command: name, Status: CheckAllowed}
			c.commandPolicy(child, false, state.pipeline)
			entry.Status = combineCheckStatus(entry.Status, child.Status)
			entry.Issues = append(entry.Issues, child.Issues...)
		}
		entry.issue(CheckRequiresExecution, "xargs builds command arguments from runtime input")
		return
	case "find":
		// find uses a separate expression parser. Check its start paths and
		// leave predicates (including -exec/-execdir) explicitly unresolved.
		operation = allowedpaths.PathLstat
	options:
		for len(paths) > 0 {
			switch paths[0] {
			case "--help":
				return
			case "-H":
				entry.issue(CheckInvalidArguments, "find: -H is not supported")
				return
			case "-L":
				operation = allowedpaths.PathRead
			case "-P":
				operation = allowedpaths.PathLstat
			case "--":
				paths = paths[1:]
				break options
			default:
				break options
			}
			paths = paths[1:]
		}
		end := 0
		for end < len(paths) && !(strings.HasPrefix(paths[end], "-") && len(paths[end]) > 1) && paths[end] != "!" && paths[end] != "(" {
			end++
		}
		if end < len(paths) {
			entry.issue(CheckRequiresExecution, "find predicates and nested commands require a runtime check")
		}
		paths, stdinDash = paths[:end], false
		if len(paths) == 0 {
			paths = []string{"."}
		}
	case "systemctl":
		c.checkSystemctl(entry, operands)
		return
	case "journalctl":
		c.checkJournalctl(entry, flags, operands)
		return
	default:
		entry.issue(CheckRequiresExecution, "builtin operand policies are not statically modeled")
		return
	}
	for _, path := range paths {
		if c.ctx.Err() != nil {
			return
		}
		if stdinDash && path == "-" {
			continue
		}
		c.path(entry, path, operation, state)
	}
}

func (c *scriptChecker) checkSystemctl(entry *CommandCheck, args []string) {
	verb := "list-units"
	if len(args) > 0 {
		verb, args = args[0], args[1:]
	}
	if verb == "list-units" {
		// Enumeration itself is bounded to the exact read-granted unit names.
		if len(args) > 0 {
			entry.issue(CheckInvalidArguments, "systemctl list-units: operands are not supported")
		}
		return
	}
	action := SystemServiceAction(verb)
	if verb == "status" {
		action = SystemServiceRead
	} else if !validSystemServiceAction(action) || action == SystemServiceClean || action == SystemServiceRead {
		entry.issue(CheckInvalidArguments, "systemctl: unsupported command "+verb)
		return
	}
	if len(args) == 0 {
		entry.issue(CheckInvalidArguments, "systemctl: at least one exact unit is required")
		return
	}
	for _, unit := range args {
		if err := c.policy.systemServices.authorize(c.policy.remediationMode, SystemdOperation{Service: unit, Action: action}); err != nil {
			entry.issue(CheckSystemServiceNotAllowed, err.Error())
		}
	}
}

func (c *scriptChecker) checkJournalctl(entry *CommandCheck, flags *builtins.FlagSet, args []string) {
	if len(args) > 0 {
		entry.issue(CheckInvalidArguments, "journalctl: arbitrary journal matches are not supported")
		return
	}
	units, _ := flags.GetStringArray("unit")
	action := SystemServiceRead
	maintenance := checkFlag(flags, "rotate") || flags.Changed("vacuum-size") || flags.Changed("vacuum-time") || checkFlag(flags, "dry-run")
	if maintenance {
		if !c.policy.remediationMode {
			entry.issue(CheckRemediationRequired, "journal maintenance requires remediation mode")
			return
		}
		action = SystemServiceClean
	}
	if checkFlag(flags, "dmesg") || checkFlag(flags, "disk-usage") || maintenance {
		units = []string{builtins.SystemdJournaldService}
	}
	if len(units) == 0 {
		entry.issue(CheckInvalidArguments, "journalctl: an exact --unit or --dmesg scope is required")
		return
	}
	for _, unit := range units {
		if err := c.policy.systemServices.authorize(c.policy.remediationMode, SystemdOperation{Service: unit, Action: action}); err != nil {
			entry.issue(CheckSystemServiceNotAllowed, err.Error())
		}
	}
}
