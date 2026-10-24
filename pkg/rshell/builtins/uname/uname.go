// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package uname implements the uname builtin command.
//
// uname — print system information
//
// Usage: uname [-asnrvm] [--help]
//
// Print certain system information. With no flags, same as -s.
//
// Reads system information from /proc/sys/kernel/ pseudo-files via the
// ProcProvider. The proc path is configurable via the --proc-path CLI
// flag or interp.ProcPath() API option (e.g., /host/proc for containers).
//
// Flags:
//
//	-s  Print the kernel name (default when no flags given)
//	-n  Print the network node hostname
//	-r  Print the kernel release
//	-v  Print the kernel version
//	-m  Print the machine hardware name
//	-a  Print all of the above, in the order shown
//	-h, --help  Display help and exit
//
// Data sources (relative to proc path):
//
//	-s  sys/kernel/ostype
//	-n  sys/kernel/hostname
//	-r  sys/kernel/osrelease
//	-v  sys/kernel/version
//	-m  sys/kernel/arch
//
// Exit codes:
//
//	0  Success — requested information was written.
//	1  Error — unsupported platform, missing proc file, or invalid flag.
package uname

import (
	"context"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the uname builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "uname",
	Description: "print system information",
	MakeFlags:   makeFlags,
}

// kernelFiles maps each flag letter to the proc pseudo-file that
// provides the corresponding value. Order matches POSIX -a output.
var kernelFiles = [...]struct {
	short string
	long  string
	file  string
}{
	{"s", "kernel-name", "ostype"},
	{"n", "nodename", "hostname"},
	{"r", "kernel-release", "osrelease"},
	{"v", "kernel-version", "version"},
	{"m", "machine", "arch"}, // Available since Linux 2.6 (2003); last 2.6 LTS (2.6.32) EOL Feb 2016.
}

func makeFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.BoolP("help", "h", false, "print usage and exit")
	var flags [len(kernelFiles)]*bool
	for i, entry := range kernelFiles {
		flags[i] = fs.BoolP(entry.long, entry.short, false, "")
	}
	allFlag := fs.BoolP("all", "a", false, "print all information")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			callCtx.Outf("Usage: uname [OPTION]...\n")
			callCtx.Outf("Print system information. With no OPTION, same as -s.\n\n")
			callCtx.Outf("  -s, --kernel-name     print the kernel name\n")
			callCtx.Outf("  -n, --nodename        print the network node hostname\n")
			callCtx.Outf("  -r, --kernel-release  print the kernel release\n")
			callCtx.Outf("  -v, --kernel-version  print the kernel version\n")
			callCtx.Outf("  -m, --machine         print the machine hardware name\n")
			callCtx.Outf("  -a, --all             print all information\n")
			callCtx.Outf("      --help            display this help and exit\n")
			return builtins.Result{}
		}

		if len(args) > 0 {
			callCtx.Errf("uname: extra operand '%s'\n", builtins.SafeOperand(args[0]))
			callCtx.Errf("Try 'uname --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		if runtime.GOOS != "linux" {
			callCtx.Errf("uname: not supported on %s (Linux only)\n", runtime.GOOS)
			return builtins.Result{Code: 1}
		}

		if callCtx.Proc == nil {
			callCtx.Errf("uname: not supported (no proc filesystem configured)\n")
			return builtins.Result{Code: 1}
		}

		// Default: -s when no flags given.
		anySet := *allFlag
		if !anySet {
			for _, f := range flags {
				if *f {
					anySet = true
					break
				}
			}
		}
		if !anySet {
			*flags[0] = true // -s
		}

		var parts []string
		for i, entry := range kernelFiles {
			if !*allFlag && !*flags[i] {
				continue
			}
			if ctx.Err() != nil {
				return builtins.Result{Code: 1}
			}
			val, err := callCtx.Proc.ReadKernelFile(entry.file)
			if err != nil {
				callCtx.Errf("uname: cannot read %s: %s\n", entry.file, err)
				return builtins.Result{Code: 1}
			}
			parts = append(parts, val)
		}

		callCtx.Outf("%s\n", strings.Join(parts, " "))
		return builtins.Result{}
	}
}
