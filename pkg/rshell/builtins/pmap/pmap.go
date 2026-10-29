// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pmap implements the pmap builtin command.
//
// pmap — report per-process virtual memory mappings
//
// Usage: pmap [-x] [-h|--help] PID...
//
// Display the virtual memory mappings of one or more running processes:
// start address, size, permission mode, and a mapping label (a file base
// name, a bracketed special region such as "[heap]"/"[stack]", or
// "[ anon ]" for anonymous private memory).
//
// Mapping enumeration is delegated to the internal procmaps package via
// callCtx.Proc, which reads <ProcPath>/<pid>/maps (or smaps for -x) on Linux
// directly, bypassing the AllowedPaths sandbox — the same documented
// exception the ss, ip route, df, free, and ps builtins use. ProcPath is
// fixed by the embedding application and the remaining path is derived only
// from the numeric PID, never arbitrary script input. On Windows, mappings
// are enumerated with VirtualQueryEx. On macOS, mappings are enumerated via
// proc_pidinfo(PROC_PIDREGIONINFO) (see the procmaps package doc comment for
// details).
//
// The header line printed before each process's mappings shows its short
// comm/executable name, never its full command line — pmap does not read
// or expose process argv, matching the same restriction the ps builtin
// already enforces.
//
// Accepted flags:
//
//	-x, --extended
//	    Extended format: also show per-mapping RSS and Dirty (Linux only;
//	    exits 1 with "not supported" on Windows and macOS rather than
//	    reporting fabricated zeros).
//
//	-h, --help
//	    Print usage to stdout and exit 0.
//
// Rejected flags (intentionally not registered, rejected as unknown by
// pflag with exit 1): -p/--show-path (full mapping paths), -d/--device
// (device format), -q/--quiet, -A/--range, -k/--use-kernel-name,
// -c/--read-rc, -C/--read-rc-from, -n/--create-rc,
// -N/--create-rc-to (rc-file-driven filtering), -X/-XX (kernel-footprint
// extended stats), and -V/--version. pmap is scoped to a single read-only
// snapshot per PID; deferred to a later version if evidenced.
//
// Exit codes:
//
//	0  Success — every requested PID's mappings were written.
//	1  Error — invalid PID, unsupported platform/mode, a PID that does not
//	   name a running process, or no PID given.
package pmap

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procmaps"
)

// Cmd is the pmap builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "pmap",
	Description: "report per-process virtual memory mappings",
	MakeFlags:   makeFlags,
}

// noArgSentinel is the NoOptDefVal used for --help and -x/--extended so that
// explicit-value forms (--extended=true) are rejected, matching GNU getopt's
// no-argument behaviour. See builtins/df/df.go's noArgBool for the full
// rationale: a NUL byte cannot appear in argv (execve rejects it), so any
// non-sentinel value passed to Set means the user wrote "=value" and must
// be refused.
const noArgSentinel = "\x00"

// noArgBool mirrors df.noArgBool. Duplicated locally (rather than shared)
// because it is a small, self-contained pflag.Value and df's copy is
// unexported.
type noArgBool struct {
	target *bool
}

func (b *noArgBool) String() string {
	if b.target != nil && *b.target {
		return "true"
	}
	return "false"
}
func (b *noArgBool) Type() string { return "bool" }
func (b *noArgBool) Set(s string) error {
	if s != noArgSentinel {
		return errors.New("flag does not allow an argument")
	}
	*b.target = true
	return nil
}

func registerNoArgBool(fs *builtins.FlagSet, name, shorthand, usage string) *bool {
	target := new(bool)
	flag := fs.VarPF(&noArgBool{target: target}, name, shorthand, usage)
	flag.NoOptDefVal = noArgSentinel
	return target
}

func makeFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := registerNoArgBool(fs, "help", "h", "print usage and exit")
	extended := registerNoArgBool(fs, "extended", "x", "show RSS and Dirty per mapping (Linux only)")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}

		if len(args) == 0 {
			callCtx.Errf("pmap: no process ID specified\n")
			callCtx.Errf("Try 'pmap --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		pids, err := parsePIDs(args)
		if err != nil {
			callCtx.Errf("pmap: %v\n", err)
			return builtins.Result{Code: 1}
		}

		hadError := false
		for _, pid := range pids {
			if ctx.Err() != nil {
				return builtins.Result{Code: 1}
			}
			if err := printProcess(ctx, callCtx, pid, *extended); err != nil {
				if ctx.Err() != nil {
					return builtins.Result{Code: 1}
				}
				hadError = true
				switch {
				case errors.Is(err, procmaps.ErrNoSuchProcess):
					callCtx.Errf("pmap: %d: no such process\n", pid)
				case errors.Is(err, procmaps.ErrNotSupported):
					callCtx.Errf("pmap: not supported on this platform\n")
				case errors.Is(err, procmaps.ErrExtendedNotSupported):
					callCtx.Errf("pmap: -x is not supported on this platform\n")
				default:
					callCtx.Errf("pmap: %v\n", err)
				}
			}
		}

		if hadError {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

// parsePIDs validates that every argument is a positive integer PID.
func parsePIDs(args []string) ([]int, error) {
	pids := make([]int, 0, len(args))
	for _, a := range args {
		pid, err := strconv.Atoi(a)
		if err != nil || pid <= 0 {
			return nil, fmt.Errorf("invalid PID: %s", a)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// printProcess writes one process's header line, mapping rows, and total
// line to stdout.
func printProcess(ctx context.Context, callCtx *builtins.CallContext, pid int, extended bool) error {
	name, mappings, err := callCtx.Proc.ReadMaps(ctx, pid, extended)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, _, _, ok := mappingTotals(mappings); !ok {
		return errors.New("mapping totals overflow")
	}

	callCtx.Outf("%d:   %s\n", pid, name)

	if extended {
		printExtended(callCtx, mappings)
	} else {
		printBasic(callCtx, mappings)
	}
	return nil
}

func printBasic(callCtx *builtins.CallContext, mappings []procmaps.Mapping) {
	totalKB, _, _, _ := mappingTotals(mappings)
	for _, m := range mappings {
		size := m.SizeKB()
		callCtx.Outf("%016x %6dK %s %s\n", m.Start, size, m.Perms, m.Name)
	}
	callCtx.Outf(" total %13dK\n", totalKB)
}

func printExtended(callCtx *builtins.CallContext, mappings []procmaps.Mapping) {
	callCtx.Outf("Address           Kbytes     RSS   Dirty Mode  Mapping\n")
	totalKB, totalRSS, totalDirty, _ := mappingTotals(mappings)
	for _, m := range mappings {
		size := m.SizeKB()
		callCtx.Outf("%016x %7d %7d %7d %s %s\n", m.Start, size, m.RSSKB, m.DirtyKB, m.Perms, m.Name)
	}
	callCtx.Outf("---------------- ------- ------- -------\n")
	callCtx.Outf("total kB         %7d %7d %7d\n", totalKB, totalRSS, totalDirty)
}

func mappingTotals(mappings []procmaps.Mapping) (size, rss, dirty uint64, ok bool) {
	for _, m := range mappings {
		mappingSize := m.SizeKB()
		if mappingSize > ^uint64(0)-size || m.RSSKB > ^uint64(0)-rss || m.DirtyKB > ^uint64(0)-dirty {
			return 0, 0, 0, false
		}
		size += mappingSize
		rss += m.RSSKB
		dirty += m.DirtyKB
	}
	return size, rss, dirty, true
}

// printHelp emits the help text to stdout (per RULES.md, help is not an
// error; exit 0 with output on stdout). Mirrors df's NoOptDefVal-clearing
// dance so --help doesn't render a literal NUL byte for the no-argument
// flags.
func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: pmap [-x] [-h|--help] PID...\n")
	callCtx.Out("Report the virtual memory mappings of one or more processes.\n\n")
	saved := make(map[*builtins.Flag]string)
	fs.VisitAll(func(flag *builtins.Flag) {
		if flag.NoOptDefVal == noArgSentinel {
			saved[flag] = flag.NoOptDefVal
			flag.NoOptDefVal = ""
		}
	})
	defer func() {
		for f, v := range saved {
			f.NoOptDefVal = v
		}
	}()
	fs.SetOutput(callCtx.Stdout)
	fs.PrintDefaults()
}
