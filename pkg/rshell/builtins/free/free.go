// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package free implements the free builtin command.
//
// free — report host memory and swap usage
//
// Usage: free [-h] [--help]
//
// Display a snapshot of total, used, free, shared, buffer/cache, and
// available memory, plus swap usage. This is a narrow read-only
// investigation builtin for confirming host-level memory pressure — it is
// not a remediation command and does not sample repeatedly; repeated
// sampling and trend interpretation are out of scope (see vmstat).
//
// Memory enumeration is delegated to the internal meminfo package, which
// reads /proc/meminfo on Linux. The /proc read is exempt from the
// AllowedPaths sandbox because the path is hardcoded and never derived
// from user input — the same documented exception the ss, ip route, and
// df builtins use. Linux only; macOS and Windows exit 1 with "not
// supported on this platform" (see the meminfo package doc comment for
// why).
//
// Accepted flags:
//
//	-h, --human
//	    Print sizes in human-readable powers-of-1024 form with an IEC
//	    binary suffix (e.g. 1.5Gi), matching GNU free's -h.
//
//	--help
//	    Print usage to stdout and exit 0. No short form: -h is already
//	    used for human-readable output, matching GNU free.
//
// Rejected flags (intentionally not registered, rejected as unknown by
// pflag with exit 1): -b/-k/-m/-g/--si/--tera/--peta unit selectors, -w/
// --wide, -t/--total, -l/--lohi, and -s/--seconds + -c/--count repeated
// sampling. free is scoped to a single-shot snapshot; deferred to a later
// version if evidenced.
//
// Exit codes:
//
//	0  Success — the memory/swap snapshot was written.
//	1  Error — unsupported platform, unknown flag, extra operand, or
//	   failure to read /proc/meminfo.
package free

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/meminfo"
)

// Cmd is the free builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "free",
	Description: "report host memory and swap usage",
	MakeFlags:   makeFlags,
}

// noArgSentinel is the NoOptDefVal used for -h/--human and --help so that
// explicit-value forms (--human=true) are rejected, matching GNU getopt's
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
	help := registerNoArgBool(fs, "help", "", "print usage and exit")
	human := registerNoArgBool(fs, "human", "h", "print sizes in human-readable form (e.g. 1.5Gi)")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}

		if len(args) > 0 {
			callCtx.Errf("free: extra operand '%s'\n", builtins.SafeOperand(args[0]))
			callCtx.Errf("Try 'free --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		info, err := meminfo.Read(ctx)
		switch {
		case errors.Is(err, meminfo.ErrNotSupported):
			callCtx.Errf("free: not supported on this platform\n")
			return builtins.Result{Code: 1}
		case err != nil:
			callCtx.Errf("free: %v\n", err)
			return builtins.Result{Code: 1}
		}

		if err := ctx.Err(); err != nil {
			return builtins.Result{Code: 1}
		}

		writeOutput(callCtx, info, *human)
		return builtins.Result{}
	}
}

// memRow and swapRow hold the columns for the two output lines. Swap has
// no shared/buffCache/available counterpart, so it only fills total/used/
// free.
type memRow struct {
	total, used, free, shared, buffCache, available string
}

type swapRow struct {
	total, used, free string
}

// writeOutput computes the derived columns and prints the two-row table.
//
// buff/cache is Buffers + Cached + SReclaimable (reclaimable slab memory
// counts toward the cache column in modern free). used follows GNU free's
// actual formula, MemTotal - MemAvailable, not the naive
// Total-Free-BuffCache: under cgroup/container memory accounting,
// MemAvailable can be lower than MemFree+buff/cache, and free's used
// column tracks that lower ceiling rather than the raw free+cache
// subtraction (verified against /usr/bin/free on Linux — for the same
// host snapshot shape, the naive formula under-reports used by ~500MiB).
func writeOutput(callCtx *builtins.CallContext, info meminfo.Info, human bool) {
	buffCache := saturatingAdd(info.Buffers, saturatingAdd(info.Cached, info.SReclaimable))
	used := saturatingSub(info.MemTotal, info.MemAvailable)
	swapUsed := saturatingSub(info.SwapTotal, info.SwapFree)

	fmtVal := func(v uint64) string {
		if human {
			return humanBytes(v)
		}
		// Default unit is KiB, matching GNU free. Every input value here
		// is an exact multiple of 1024 (meminfo converts /proc/meminfo's
		// native kB unit to bytes via ×1024, and used/buffCache/swapUsed
		// are sums/differences of such values), so integer division
		// recovers the original kernel-reported KiB count losslessly.
		return strconv.FormatUint(v/1024, 10)
	}

	mem := memRow{
		total:     fmtVal(info.MemTotal),
		used:      fmtVal(used),
		free:      fmtVal(info.MemFree),
		shared:    fmtVal(info.Shared),
		buffCache: fmtVal(buffCache),
		available: fmtVal(info.MemAvailable),
	}
	swap := swapRow{
		total: fmtVal(info.SwapTotal),
		used:  fmtVal(swapUsed),
		free:  fmtVal(info.SwapFree),
	}

	printRows(callCtx, mem, swap)
}

// printRows renders the header and two data rows with columns aligned to
// the widest cell, mirroring df's printRows column-alignment approach.
func printRows(callCtx *builtins.CallContext, mem memRow, swap swapRow) {
	headers := []string{"total", "used", "free", "shared", "buff/cache", "available"}
	memCells := []string{mem.total, mem.used, mem.free, mem.shared, mem.buffCache, mem.available}
	swapCells := []string{swap.total, swap.used, swap.free}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for i, c := range memCells {
		widths[i] = max(widths[i], len(c))
	}
	for i, c := range swapCells {
		widths[i] = max(widths[i], len(c))
	}

	labelWidth := max(len("Mem:"), len("Swap:"))

	var out []byte
	out = append(out, fmtLabel("", labelWidth)...)
	for i, h := range headers {
		out = append(out, ' ')
		out = append(out, fmtRight(h, widths[i])...)
	}
	out = append(out, '\n')

	out = append(out, fmtLabel("Mem:", labelWidth)...)
	for i, c := range memCells {
		out = append(out, ' ')
		out = append(out, fmtRight(c, widths[i])...)
	}
	out = append(out, '\n')

	out = append(out, fmtLabel("Swap:", labelWidth)...)
	for i, c := range swapCells {
		out = append(out, ' ')
		out = append(out, fmtRight(c, widths[i])...)
	}
	out = append(out, '\n')

	callCtx.Out(string(out))
}

func fmtLabel(s string, width int) []byte {
	b := []byte(s)
	for len(b) < width {
		b = append(b, ' ')
	}
	return b
}

func fmtRight(s string, width int) []byte {
	pad := width - len(s)
	b := make([]byte, 0, width)
	for range pad {
		b = append(b, ' ')
	}
	b = append(b, s...)
	return b
}

// humanBytes formats a byte count using IEC binary prefixes (Ki, Mi, Gi,
// Ti, Pi, Ei), matching GNU free's -h output (which differs from GNU df's
// -h: df uses bare K/M/G, free uses the "i" IEC suffix).
//
// Below 10 (one decimal shown), this formats the scaled value directly
// with fmt.Sprintf("%.1f"), the same thing procps-ng's own scale_size()
// does with plain printf: both Go's fmt and C's printf apply IEEE 754
// round-half-to-even for exact decimal ties, so formatting directly
// reproduces free's tie-breaking exactly (e.g. 1310720 bytes is exactly
// 1.25Mi, which free prints as "1.2Mi", not "1.3Mi"). An earlier version
// of this function pre-rounded with a custom round-half-away-from-zero
// helper, which silently diverged from free at exact ties — do not
// reintroduce a custom rounding step for the below-10 case.
//
// At 10 and above (no decimal shown), free truncates instead of rounding
// (see formatScaled), so this must not reuse "%.0f" — that rounds. Zero
// is special-cased to "0B" (no decimal), matching GNU free.
func humanBytes(v uint64) string {
	if v == 0 {
		return "0B"
	}
	suffixes := []string{"B", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei"}
	if v < 1024 {
		return strconv.FormatUint(v, 10) + "B"
	}
	val := float64(v)
	suffixIdx := 0
	for val >= 1024 && suffixIdx < len(suffixes)-1 {
		val /= 1024
		suffixIdx++
	}
	return formatScaled(val, suffixIdx, suffixes)
}

// formatScaled renders val (already scaled into [1, 1024) at suffixIdx,
// except at the largest suffix where val may still be >= 1024) using
// procps-ng's actual field-width-driven promotion rule rather than a
// hardcoded ">= 1000" or ">= 10" threshold.
//
// Real procps-ng free -h (scale_size() in local/units.c) does not promote
// exactly at the 1024 boundary of the current unit. For each candidate
// unit it first tries a one-decimal rendering ("%.1f" + 2-char suffix,
// e.g. "1.2Ki") and only accepts it if the whole string is at most 5
// characters wide; otherwise it falls back to a truncated integer
// rendering ("%lu" + suffix, e.g. "999Ki"), accepted under the same
// 5-character budget. If neither fits — which happens precisely when the
// integer part has reached 4 digits, e.g. 1010 in "1010Ki" — it escalates
// to the next unit and retries. This is why 1010*1024 bytes (1010Ki) is
// displayed as "1.0Mi", not "1010Ki": "1010Ki" is 6 characters, one over
// budget, even though 1010 hasn't reached the true 1024 boundary.
//
// Below 10, the one-decimal form always fits ("X.YKi" is 5 chars), so
// free shows one decimal place there, rounded IEEE 754 round-half-to-even
// (matching printf's "%.1f" and Go's fmt) — e.g. 1310720 bytes is exactly
// 1.25Mi, which free prints as "1.2Mi", not "1.3Mi". Do not reintroduce a
// custom round-half-away-from-zero helper for this case.
//
// From 10 up to 999, the one-decimal form no longer fits (e.g. "99.9Ki"
// is 6 chars), so free falls back to the truncated integer form — e.g. a
// 69133532 KiB total is 65.926 GiB, and /usr/bin/free -h prints "65Gi",
// not "66Gi" (which rounding would produce).
//
// A rendering that lands on exactly 1024 (or, for the integer form, a
// truncated value that reached 1024) still promotes to the next suffix,
// e.g. 1023.95Ki must read as "1.0Mi", not the awkward "1024.0Ki".
func formatScaled(val float64, suffixIdx int, suffixes []string) string {
	for {
		suffix := suffixes[suffixIdx]
		atLastSuffix := suffixIdx == len(suffixes)-1

		decimal := fmt.Sprintf("%.1f", val)
		if len(decimal)+len(suffix) <= 5 {
			if rounded, err := strconv.ParseFloat(decimal, 64); err == nil && rounded >= 1024 && !atLastSuffix {
				val = rounded / 1024
				suffixIdx++
				continue
			}
			return decimal + suffix
		}

		truncated := uint64(val)
		integer := strconv.FormatUint(truncated, 10)
		if len(integer)+len(suffix) <= 5 {
			if truncated >= 1024 && !atLastSuffix {
				val = float64(truncated) / 1024
				suffixIdx++
				continue
			}
			return integer + suffix
		}

		// Neither the one-decimal nor the truncated-integer rendering
		// fits the 5-character field width at this unit (e.g. val is
		// 1010, "1010Ki" is 6 chars): escalate to the next unit and
		// retry. At the largest suffix there is nowhere left to
		// escalate to, so fall back to the (oversized) integer form.
		if atLastSuffix {
			return integer + suffix
		}
		val /= 1024
		suffixIdx++
	}
}

func saturatingAdd(a, b uint64) uint64 {
	if a > ^uint64(0)-b {
		return ^uint64(0)
	}
	return a + b
}

func saturatingSub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

// printHelp emits the help text to stdout (per RULES.md, help is not an
// error; exit 0 with output on stdout). Mirrors df's NoOptDefVal-clearing
// dance so --help doesn't render a literal NUL byte for the no-argument
// flags.
func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: free [-h] [--help]\n")
	callCtx.Out("Display total, used, free, shared, buffer/cache, and available\n")
	callCtx.Out("memory, plus swap usage.\n\n")
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
