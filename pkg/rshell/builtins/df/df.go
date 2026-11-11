// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package df implements the df builtin command.
//
// df — report file system disk space usage
//
// Usage: df [OPTION]...
//
// Show information about the file system on which each FILE resides, or
// all file systems by default. This implementation does not accept FILE
// operands; pipe through grep to filter.
//
// Mount enumeration is delegated to the internal diskstats package, which
// reads /proc/self/mountinfo on Linux and calls getfsstat(2) on macOS.
// The /proc read is exempt from the AllowedPaths sandbox because the path
// is hardcoded and never derived from user input — the same documented
// exception the ss and ip route builtins use.
//
// Accepted flags:
//
//	-h, --human-readable
//	    Print sizes in powers of 1024 (e.g. 1023M, 1.5G).
//
//	-H, --si
//	    Print sizes in powers of 1000 (e.g. 1.1G).
//
//	-k
//	    Use 1024-byte blocks (POSIX default; the column header reads
//	    "1K-blocks").
//
//	-P, --portability
//	    Use the POSIX output format. Single space-separated header line:
//	    "Filesystem 1024-blocks Used Available Capacity Mounted on".
//
//	-T, --print-type
//	    Add a column showing the filesystem type.
//
//	-i, --inodes
//	    List inode usage instead of block usage.
//
//	-a, --all
//	    Include pseudo, duplicate, and inaccessible filesystems.
//
//	-t, --type=TYPE
//	    Limit the listing to filesystems of TYPE. May be repeated.
//
//	-x, --exclude-type=TYPE
//	    Exclude filesystems of TYPE. May be repeated. If TYPE is named
//	    by both -t and -x, exclusion wins.
//
//	-l, --local
//	    Limit the listing to local filesystems.
//
//	--total
//	    Append a grand-total row.
//
//	--no-sync
//	    Accepted as a no-op (this is the default behaviour).
//
//	--help
//	    Print usage to stdout and exit 0.
//
// Rejected flags (intentionally not registered, rejected as unknown by
// pflag with exit 1):
//
//	--sync   — invokes sync(2), modifying kernel buffer state. Violates
//	           the "no system state mutation" rule.
//	-B/--block-size, --output, [FILE]…   — deferred to a later version.
//	-v, --version                        — not meaningful in this shell.
//
// Exit codes:
//
//	0  Success — listing was written.
//	1  Error — unsupported platform, unknown flag, extra operand, or
//	   failure to enumerate the mount table.
package df

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/diskstats"
)

// Cmd is the df builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "df",
	Description: "report file system disk space usage",
	MakeFlags:   makeFlags,
}

// unitMode controls how byte counts are formatted in block columns.
type unitMode int

const (
	unitsK         unitMode = iota // 1024-byte blocks (POSIX default)
	unitsHuman1024                 // -h: powers of 1024
	unitsHuman1000                 // -H: powers of 1000
)

// noArgSentinel is the NoOptDefVal we set on every no-argument flag
// (unitFlag and noArgBool). pflag passes this exact string to Set when
// the user writes the bare flag (`-h`, `--all`); for the explicit-value
// form (`--all=true`) pflag passes the user's literal value instead.
//
// We use a single NUL byte so the two cases are distinguishable: NUL
// is the C-string terminator and POSIX execve(2) refuses to pass argv
// elements containing it, so the user cannot forge this string from a
// shell. That lets Set reject every `=value` form — including `=true`
// — to match GNU df's "doesn't allow an argument" exit-1 error.
const noArgSentinel = "\x00"

// unitFlag is a pflag.Value that writes a fixed unitMode into a shared
// target each time the flag is set. We use one instance for -h (writes
// unitsHuman1024) and one for -H (writes unitsHuman1000) sharing a
// pointer to the same `mode` field — the LAST set wins by overwriting,
// which is exactly the argv-order semantics GNU df documents.
//
// pflag.FlagSet.Visit walks set flags in *lexicographical* order, not
// argv order, so it cannot be used to honor input ordering. A
// shared-target Var sidesteps that limitation entirely.
type unitFlag struct {
	target *unitMode
	value  unitMode
}

func (u *unitFlag) String() string { return "" }
func (u *unitFlag) Type() string   { return "bool" }

// Set is called by pflag once per occurrence of the flag. It receives:
//   - noArgSentinel for a bare flag (e.g. `-h`, `--human-readable`);
//   - the user's literal value for `--name=value` / `-name=value`.
//
// GNU df rejects every explicit-value form (`gdf --human-readable=false`
// errors with "option '--human-readable' doesn't allow an argument";
// even `=true` is rejected). The sentinel is unforgeable from a shell
// argv (NUL bytes can't be passed through execve), so any non-sentinel
// value here means the user wrote `--flag=value` and must be rejected.
func (u *unitFlag) Set(s string) error {
	if s != noArgSentinel {
		return errors.New("flag does not allow an argument")
	}
	*u.target = u.value
	return nil
}

// registerUnitFlag installs a unitFlag at name/shorthand and configures
// NoOptDefVal so users can pass `-h` / `-H` (no argument). Without
// NoOptDefVal, pflag treats Var-registered flags as requiring a value
// and rejects `-h` with "flag needs an argument".
func registerUnitFlag(fs *builtins.FlagSet, target *unitMode, value unitMode, name, shorthand, usage string) {
	flag := fs.VarPF(&unitFlag{target: target, value: value}, name, shorthand, usage)
	flag.NoOptDefVal = noArgSentinel
}

// noArgBool is a pflag.Value that mirrors GNU getopt's no_argument
// behaviour: bare `--flag` and `-f` work, but `--flag=value` and
// `-f=value` are rejected with "flag does not allow an argument" for
// every value (including `=true`). pflag.BoolP treats the
// explicit-value form as a successful parse, which silently diverges
// from GNU.
//
// Like unitFlag, this relies on noArgSentinel (a NUL byte) to
// distinguish a bare flag from an explicit `=value`.
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

// registerNoArgBool installs a noArgBool flag and returns the *bool
// target so the caller can read it like an ordinary fs.Bool result.
// Pass an empty shorthand for long-only flags (e.g. --total).
func registerNoArgBool(fs *builtins.FlagSet, name, shorthand, usage string) *bool {
	target := new(bool)
	flag := fs.VarPF(&noArgBool{target: target}, name, shorthand, usage)
	flag.NoOptDefVal = noArgSentinel
	return target
}

// flags carries the parsed flag state. It is constructed once per
// invocation by makeFlags and consumed by the bound handler.
type flags struct {
	help         *bool
	mode         *unitMode // updated by the unitFlag values for -h / -H
	posix        *bool
	printType    *bool
	inodes       *bool
	all          *bool
	local        *bool
	total        *bool
	noSync       *bool
	includeTypes *[]string
	excludeTypes *[]string
}

func makeFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	mode := unitsK
	// All boolean options use registerNoArgBool so explicit-value
	// forms (`--all=false`, `--portability=false`, etc.) are
	// rejected with "flag does not allow an argument" — matching GNU
	// df's getopt(1)-style refusal for no-argument options.
	f := &flags{
		help:         registerNoArgBool(fs, "help", "", "print usage and exit"),
		mode:         &mode,
		posix:        registerNoArgBool(fs, "portability", "P", "use the POSIX output format"),
		printType:    registerNoArgBool(fs, "print-type", "T", "print file system type"),
		inodes:       registerNoArgBool(fs, "inodes", "i", "list inode information instead of block usage"),
		all:          registerNoArgBool(fs, "all", "a", "include pseudo, duplicate, inaccessible file systems"),
		local:        registerNoArgBool(fs, "local", "l", "limit listing to local file systems"),
		total:        registerNoArgBool(fs, "total", "", "append a grand total row"),
		noSync:       registerNoArgBool(fs, "no-sync", "", "do not invoke sync before getting usage info (default; accepted for compatibility)"),
		includeTypes: fs.StringArrayP("type", "t", nil, "limit listing to file systems of type TYPE"),
		excludeTypes: fs.StringArrayP("exclude-type", "x", nil, "limit listing to file systems not of type TYPE"),
	}
	// -h / -H / -k all share `mode` via unitFlag so argv order picks
	// the winner (last-set wins). See unitFlag's doc for the
	// rationale; including -k here matches GNU df, where
	// `df -h -k` prints "1K-blocks" because -k overrides the earlier
	// -h, and `df -k -h` prints "Size" for the reverse reason.
	//
	// -k is registered with shorthand only because GNU df has no
	// long form for it (the GNU manual documents -k as "equivalent
	// to --block-size=1K"; no --kibibytes long flag exists). Adding
	// a long form would let scripts depend on rshell-only behavior.
	registerUnitFlag(fs, &mode, unitsHuman1024, "human-readable", "h", "print sizes in powers of 1024 (e.g. 1023M)")
	registerUnitFlag(fs, &mode, unitsHuman1000, "si", "H", "print sizes in powers of 1000 (e.g. 1.1G)")
	// -k has no long form (GNU documents -k as the only spelling).
	// pflag.PrintDefaults can't render a shorthand-only flag — it
	// would emit "-k, --" with an empty long name — so we hide it
	// from the auto-generated help and handle the line manually in
	// printHelp.
	kFlag := fs.VarPF(&unitFlag{target: &mode, value: unitsK}, "", "k", "use 1024-byte blocks (POSIX default)")
	kFlag.NoOptDefVal = noArgSentinel
	kFlag.Hidden = true

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *f.help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}

		if len(args) > 0 {
			callCtx.Errf("df: extra operand '%s'\n", builtins.SafeOperand(args[0]))
			callCtx.Errf("Try 'df --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		// GNU df: a type appearing in both -t and -x is a usage
		// error, not a silent "exclude wins" — surface it before any
		// other work so configs / scripts that accidentally name the
		// same type in both lists fail loudly.
		if dup := overlappingType(*f.includeTypes, *f.excludeTypes); dup != "" {
			callCtx.Errf("df: file system type '%s' both selected and excluded\n", builtins.SafeOperand(dup))
			return builtins.Result{Code: 1}
		}

		// Pre-stat filter: drop mounts the caller already asked to
		// exclude before diskstats.List calls statfs(2) on them.
		// statfs on a stale NFS or CIFS mount can hang indefinitely
		// and is not interrupted by ctx cancellation, so `df -l` /
		// `df -x nfs` MUST decide "skip this mount" before the syscall
		// is issued. Filters that depend on capacity numbers are still
		// applied post-stat by filterMounts.
		preStat := makePreStatFilter(f)

		mounts, err := diskstats.List(ctx, preStat)
		switch {
		case errors.Is(err, diskstats.ErrMaxMounts):
			// Non-fatal: print what we have, warn on stderr.
			callCtx.Errf("df: warning: mount table truncated at %d entries\n", diskstats.MaxMounts)
		case errors.Is(err, diskstats.ErrNotSupported):
			callCtx.Errf("df: not supported on this platform\n")
			return builtins.Result{Code: 1}
		case err != nil:
			callCtx.Errf("df: %v\n", err)
			return builtins.Result{Code: 1}
		}

		// Capture whether any explicit type filter was given so we can
		// distinguish "filters left no rows" (a usage error per GNU df)
		// from "no mounts at all" (still success).
		filterRequested := len(*f.includeTypes) > 0 || len(*f.excludeTypes) > 0

		// Preserve the kernel-reported order (mountinfo on Linux,
		// getfsstat on macOS). GNU df does not sort: it walks the
		// mount table as the kernel returned it, so /dev appears
		// before /System/Volumes/* on macOS and /proc before /dev
		// on most Linux hosts. Sorting alphabetically breaks scripts
		// that compare row order against /usr/bin/df.
		mounts = filterMounts(mounts, f)

		if err := ctx.Err(); err != nil {
			return builtins.Result{Code: 1}
		}

		// GNU df: if -t/-x leaves no rows, exit 1 with a stderr
		// message. Scripts use this exit status to test filesystem
		// presence, so silently exiting 0 would be a regression.
		if filterRequested && len(mounts) == 0 {
			callCtx.Errf("df: no file systems processed\n")
			return builtins.Result{Code: 1}
		}

		writeOutput(callCtx, mounts, f, *f.mode)
		return builtins.Result{}
	}
}

// makePreStatFilter returns a diskstats.FilterFunc that drops mounts
// before they are stat(2)'d. It encodes everything filterMounts would
// have rejected based on type/pseudo/local — the categories that are
// already known from /proc/self/mountinfo at parse time and do not
// depend on capacity numbers.
//
// Running these checks pre-stat is what protects `df -l` and `df -x nfs`
// from blocking on a stale NFS mount: without it, statfs(2) is called
// on every mount in the table before any filter runs, and statfs on a
// dead remote can hang indefinitely (and is not interrupted by ctx).
//
// Filter ordering matches GNU df: -t / -x are independent of the
// pseudo / -a / -l filters. In particular, `df -t proc` does NOT
// expose pseudo proc mounts — only `-a` exempts pseudo filesystems
// from the default suppression. `df -t tmpfs` works because tmpfs is
// not in the pseudoTypes table (RAM-backed but real storage), not
// because -t overrides pseudo suppression.
func makePreStatFilter(f *flags) diskstats.FilterFunc {
	includeSet := stringSet(*f.includeTypes)
	excludeSet := stringSet(*f.excludeTypes)
	all := *f.all
	local := *f.local
	return func(m diskstats.Mount) bool {
		if _, ok := excludeSet[m.FSType]; ok {
			return false
		}
		if len(includeSet) > 0 {
			if _, ok := includeSet[m.FSType]; !ok {
				return false
			}
		}
		// Pseudo filter applies regardless of -t (-a is the only
		// flag that exposes pseudo filesystems).
		if !all && m.Pseudo {
			return false
		}
		if local && !m.Local {
			return false
		}
		return true
	}
}

// filterMounts applies post-stat filtering. The pre-stat filter
// (makePreStatFilter) has already dropped mounts that don't match
// -t/-x/-a/-l, so this pass is responsible for:
//
//  1. Deduplicating mounts that share a kernel device (matches GNU
//     df: bind-mounts of the same filesystem are elided unless -a is
//     given, and the entry with the *shortest* mount point is kept).
//     This avoids `--total` double-counting overlay / kataShared
//     bind-mounts of /etc/hosts, /etc/hostname, /etc/resolv.conf and
//     keeps the canonical mount visible (e.g. /etc/hosts rather than
//     /etc/resolv.conf).
//
// The result reuses the input slice's backing array; the caller must
// not retain the original slice after this call. diskstats.List always
// returns a fresh slice, so this is safe in the current call sites.
func filterMounts(mounts []diskstats.Mount, f *flags) []diskstats.Mount {
	if *f.all {
		// -a: keep duplicates and everything else exactly as the
		// pre-stat pass left it.
		return mounts
	}
	// First pass: per device, find the index of the entry with the
	// shortest mount point. Mounts without a DevID (rare; the
	// platform did not expose one) bypass dedup entirely and are
	// always kept.
	keep := make(map[string]int, len(mounts))
	for i, m := range mounts {
		if m.DevID == "" {
			continue
		}
		if cur, ok := keep[m.DevID]; !ok || len(m.MountPoint) < len(mounts[cur].MountPoint) {
			keep[m.DevID] = i
		}
	}
	// Second pass: emit the chosen entry (or all entries that had no
	// DevID) in the original order.
	out := mounts[:0]
	for i, m := range mounts {
		if m.DevID == "" {
			out = append(out, m)
			continue
		}
		if keep[m.DevID] == i {
			out = append(out, m)
		}
	}
	return out
}

// overlappingType returns the first type string that appears in both
// includes and excludes, or "" if the lists are disjoint. GNU df
// rejects this combination with exit 1 rather than silently letting
// exclusion win.
func overlappingType(includes, excludes []string) string {
	if len(includes) == 0 || len(excludes) == 0 {
		return ""
	}
	excl := stringSet(excludes)
	for _, t := range includes {
		if _, ok := excl[t]; ok {
			return t
		}
	}
	return ""
}

// stringSet converts the repeated -t/-x argv into a set keyed by the
// literal type strings. GNU df does NOT comma-split a single -t value;
// `df -t overlay,tmpfs` treats "overlay,tmpfs" as one literal type and
// matches nothing. Multiple types are passed as multiple -t flags. We
// match GNU exactly so scripts that rely on the no-match exit-1 path
// behave the same way under rshell.
func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	s := make(map[string]struct{}, len(values))
	for _, v := range values {
		s[v] = struct{}{}
	}
	return s
}

// row holds the formatted column values for a single mount and is shared
// by the printer and the totals accumulator.
type row struct {
	source     string
	fstype     string
	col1       string
	col2       string
	col3       string
	capacity   string
	mountpoint string
}

// writeOutput formats and prints the mount table. The columns depend on
// -P (POSIX) and -i (inodes); -T inserts an FS type column after the
// source.
func writeOutput(callCtx *builtins.CallContext, mounts []diskstats.Mount, f *flags, mode unitMode) {
	posix := *f.posix
	withType := *f.printType
	inodeMode := *f.inodes

	header := buildHeader(posix, withType, inodeMode, mode)

	var totalT, totalU, totalA uint64
	rows := make([]row, 0, len(mounts))
	for _, m := range mounts {
		t, u, a := selectColumns(m, inodeMode)
		// Totals use the raw numbers, not the formatted strings, so
		// human-mode rounding does not propagate into the grand total.
		totalT = saturatingAdd(totalT, t)
		totalU = saturatingAdd(totalU, u)
		totalA = saturatingAdd(totalA, a)
		rows = append(rows, row{
			source:     replaceUnprintable(m.Source),
			fstype:     m.FSType,
			col1:       formatCount(t, mode, inodeMode),
			col2:       formatCount(u, mode, inodeMode),
			col3:       formatCount(a, mode, inodeMode),
			capacity:   percentUsed(u, a),
			mountpoint: replaceUnprintable(m.MountPoint),
		})
	}

	if *f.total {
		rows = append(rows, row{
			source:     "total",
			fstype:     "-",
			col1:       formatCount(totalT, mode, inodeMode),
			col2:       formatCount(totalU, mode, inodeMode),
			col3:       formatCount(totalA, mode, inodeMode),
			capacity:   percentUsed(totalU, totalA),
			mountpoint: "-",
		})
	}

	// GNU df always uses an aligned column layout, even with -P (which
	// only changes header names like "Capacity" and "1024-blocks", not
	// row spacing). printRows pads each column to the width of the
	// widest cell.
	printRows(callCtx, header, rows, withType)
}

// replaceUnprintable replaces every byte in s that is non-printable
// (ASCII control character, byte 0x7F, or any byte in 0x80–0xFF that
// stands alone outside a valid UTF-8 sequence) with '?'. This matches
// the sanitization GNU df applies to keep `df` and `df -P` honoring the
// documented one-line-per-filesystem format even when a mount path or
// source contains a literal newline, tab, or other control byte. Without
// this, an attacker who can mount a filesystem at a crafted path like
// `$'/tmp/a\ndev/sda 1 1 1 1% /etc/passwd'` could inject a fake row
// into df output that scripts and AI agents parse line-by-line.
//
// The escape happens at the print boundary; the raw, decoded string
// is still passed to statfs(2) so the kernel sees the real path.
func replaceUnprintable(s string) string {
	// Fast path: most mount paths are clean ASCII. Walk the string
	// and only allocate a builder if a replacement is actually
	// needed.
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7F {
			var b strings.Builder
			b.Grow(len(s))
			b.WriteString(s[:i])
			for j := i; j < len(s); j++ {
				c := s[j]
				if c < 0x20 || c == 0x7F {
					b.WriteByte('?')
				} else {
					b.WriteByte(c)
				}
			}
			return b.String()
		}
	}
	return s
}

// selectColumns returns the (total, used, available) values that go into
// columns 1/2/3 of the listing. In inode mode they are inode counts; in
// block mode they are byte counts.
func selectColumns(m diskstats.Mount, inodeMode bool) (uint64, uint64, uint64) {
	if inodeMode {
		return m.Inodes, m.InodesUsed, m.InodesFree
	}
	return m.Total, m.Used, m.Free
}

// percentUsed renders the "Capacity" column.
//
// Edge cases:
//   - used + available == 0 → "-" (matches GNU df for empty pseudo
//     filesystems)
//   - rounds up so any non-zero usage shows ≥1%.
//
// Two scaling steps:
//
//  1. If `used + available` would overflow uint64 (which happens for
//     --total rows where each component already saturated), halve both
//     inputs first. The percentage is invariant under scaling both
//     sides by the same factor, so we lose at most one bit of
//     precision — far below the integer-percent rounding tolerance.
//     Without this step, saturatingAdd clamps the denominator to
//     MaxUint64 and equal totals get misreported as 100%.
//  2. If `used * 100` would still overflow, right-shift used and the
//     denominator together until it fits.
//
// Ceiling is computed as floor-plus-remainder-bump (rather than
// `(num + denom - 1) / denom`) because num can itself sit near MaxUint64.
func percentUsed(used, available uint64) string {
	// Remember whether usage was non-zero before any scaling; both
	// scale steps can truncate a tiny `used` to 0 (e.g. used=1 with
	// available=MaxUint64), and the "any non-zero usage rounds up to
	// ≥1%" contract must be preserved across the scale-down.
	nonzeroUsage := used > 0
	// Step 1: scale down if sum would wrap.
	if used > ^uint64(0)-available {
		used >>= 1
		available >>= 1
	}
	denom := used + available
	if denom == 0 {
		return "-"
	}
	// Step 2: shift used and denom together until used*100 fits.
	for used > (^uint64(0))/100 {
		used >>= 1
		denom >>= 1
	}
	num := used * 100
	pct := num / denom
	if num%denom != 0 {
		pct++
	}
	// If the original usage was non-zero but scaling truncated the
	// numerator to zero, restore the round-up so we don't silently
	// hide actual usage as "0%".
	if pct == 0 && nonzeroUsage {
		pct = 1
	}
	return strconv.FormatUint(pct, 10) + "%"
}

// saturatingAdd returns a + b, clamped to uint64 max on overflow. Used
// for total-row accumulation so a rogue oversized mount cannot wrap the
// running totals to zero.
func saturatingAdd(a, b uint64) uint64 {
	if a > ^uint64(0)-b {
		return ^uint64(0)
	}
	return a + b
}

// formatCount renders a numeric column.
//
// In inode mode the value is an inode count (unit-less). When -h or -H
// is also set, GNU df scales inode counts through the same K/M/G suffix
// machinery, so `df -ih` emits e.g. "4.0M" rather than "4194304". In
// non-human inode mode, the raw integer is printed.
//
// In block mode, unitsK renders the byte count divided by 1024 (1K
// blocks); the human modes call humanBytes.
func formatCount(v uint64, mode unitMode, inodeMode bool) string {
	if inodeMode {
		switch mode {
		case unitsHuman1024:
			return humanBytes(v, 1024)
		case unitsHuman1000:
			return humanBytes(v, 1000)
		}
		return strconv.FormatUint(v, 10)
	}
	switch mode {
	case unitsHuman1024:
		return humanBytes(v, 1024)
	case unitsHuman1000:
		return humanBytes(v, 1000)
	}
	// 1K blocks: round up so a non-zero value never shows as 0. Use
	// floor + remainder bump to avoid wraparound when v is near
	// MaxUint64 (totals saturate to that on overflow).
	q := v / 1024
	if v%1024 != 0 {
		q++
	}
	return strconv.FormatUint(q, 10)
}

// humanBytes formats a byte count as a power-of-base human-readable
// string. base is 1024 for -h or 1000 for -H. Output style matches GNU
// df: one decimal digit when the integer part is < 10, no decimal
// otherwise. Suffixes go up to E (exa); larger sizes are clamped at "E"
// to avoid overflow.
//
// Suffix case follows GNU df's convention: in SI / -H mode the
// kilo suffix is lowercase ("k") to match the SI symbol, while the
// kibi / -h mode keeps the uppercase "K". M, G, T, P, E stay uppercase
// in both modes — only K differs.
//
// GNU df rounds *up* on every non-integer remainder so that "Used"
// never under-reports. We mirror that with math.Ceil after scaling
// rather than fmt.Sprintf's round-to-nearest. Example: 1,576,960 bytes
// is "1.6M", not "1.5M".
//
// When the rounded value reaches `base`, it is promoted to the next
// suffix to avoid silly outputs like "1024K" — that should display as
// "1.0M". Promotion can chain (e.g. ".999...K" → "1.0M" → at the very
// top we clamp at "E" to avoid escaping the suffix table).
func humanBytes(v uint64, base uint64) string {
	suffixes := "KMGTPE"
	if base == 1000 {
		suffixes = "kMGTPE"
	}
	if v < base {
		return strconv.FormatUint(v, 10)
	}
	// Walk through suffix levels until v fits in 4 digits. The loop
	// always exits via the break before reaching the last iteration
	// for any v that fits in uint64 (max suffix level needed for
	// MaxUint64 in base-1024 is "E" at index 5, well within the
	// "KMGTPE" range), so the post-loop fallback to len(suffixes)-1
	// is unreachable in practice.
	val := float64(v)
	div := float64(base)
	suffixIdx := 0
	for i := range len(suffixes) {
		suffixIdx = i
		if val < div*float64(base) {
			break
		}
		div *= float64(base)
	}

	// Round up. The granularity depends on the pre-rounded magnitude:
	//   < 10  → one decimal place (e.g. 1.5K, 9.9G)
	//   ≥ 10  → integer (e.g. 12K, 927G)
	// This matches GNU df, which displays one decimal only for small
	// values and otherwise rounds to whole units.
	scaled := val / div
	var ceiled float64
	if scaled < 10 {
		ceiled = math.Ceil(scaled*10) / 10
	} else {
		ceiled = math.Ceil(scaled)
	}

	// Promote to the next suffix when rounding pushed the value at or
	// above the base (e.g. 1023.95K → 1024.0K → 1.0M). Without this,
	// we would emit awkward outputs like "1024K" instead of "1.0M".
	baseF := float64(base)
	if ceiled >= baseF && suffixIdx < len(suffixes)-1 {
		suffixIdx++
		ceiled /= baseF
	}

	// Final format decision uses the rounded value: 9.999K that
	// ceiling'd to 10.0K prints as "10K" with no decimal, while a
	// genuine 9.5K stays at "9.5K".
	if ceiled < 10 {
		return fmt.Sprintf("%.1f%c", ceiled, suffixes[suffixIdx])
	}
	return fmt.Sprintf("%.0f%c", ceiled, suffixes[suffixIdx])
}

// buildHeader returns the column header strings.
//
// Header naming is mode-dependent and matches GNU df verbatim:
//
//   - Block mode (default / -k / -h / -H / -P)
//   - "Capacity" appears only with strict block-POSIX (-P alone, or
//     -PT). In human modes (-h / -H) GNU keeps "Use%" even when -P
//     is also passed.
//   - Inode mode (-i, possibly with -P)
//   - The percentage column is always "IUse%". GNU keeps it that way
//     even with -iP — only the *block* POSIX format substitutes
//     "Capacity".
//   - Available column
//   - "Available" in fixed-block modes (default, -k, -P).
//   - "Avail" in human modes (-h, -H), to match GNU's compact human
//     output.
func buildHeader(posix, withType, inodeMode bool, mode unitMode) []string {
	first := "Filesystem"
	last := "Mounted on"
	human := mode == unitsHuman1024 || mode == unitsHuman1000

	if inodeMode {
		// IUse% header is preserved across -P; only the block POSIX
		// format renames the percentage column.
		cols := []string{first}
		if withType {
			cols = append(cols, "Type")
		}
		cols = append(cols, "Inodes", "IUsed", "IFree", "IUse%", last)
		return cols
	}

	// Block mode. The "Capacity" header is the strict POSIX label;
	// GNU keeps "Use%" when -P is combined with -h or -H since those
	// flags override the POSIX block-size convention.
	capacity := "Use%"
	if posix && !human {
		capacity = "Capacity"
	}

	// Size column header. -h / -H always show "Size" (the values are
	// human-suffixed), even when -P is also given — matching GNU df
	// output. The fixed-block POSIX header only applies when the unit
	// mode is itself fixed-block.
	var col1 string
	switch {
	case human:
		col1 = "Size"
	case posix:
		col1 = "1024-blocks"
	default:
		col1 = "1K-blocks"
	}

	// Available column: GNU compresses to "Avail" in human modes,
	// keeps the full "Available" in fixed-block modes.
	available := "Available"
	if human {
		available = "Avail"
	}

	cols := []string{first}
	if withType {
		cols = append(cols, "Type")
	}
	cols = append(cols, col1, "Used", available, capacity, last)
	return cols
}

// printRows emits the header row and each data row using GNU df's
// aligned column layout: every column is padded to the width of the
// widest cell in that column.
//
// We do NOT emit a strict single-space POSIX row even when -P is set.
// GNU df 9.x's `-P` documents only "one-line filesystem rows + POSIX
// header labels" — it keeps the GNU-default aligned columns. Earlier
// rshell versions emitted single-space rows for -P which diverged
// from `gdf -P` byte layout and broke bash-comparison expectations.
func printRows(callCtx *builtins.CallContext, header []string, rows []row, withType bool) {
	// Build a 2D table for column-width computation. The header is
	// always present, so len(table) is never zero.
	table := make([][]string, 0, len(rows)+1)
	table = append(table, header)
	for _, r := range rows {
		fields := []string{r.source}
		if withType {
			fields = append(fields, r.fstype)
		}
		fields = append(fields, r.col1, r.col2, r.col3, r.capacity, r.mountpoint)
		table = append(table, fields)
	}

	// Seed widths to match the minimum column widths observed in GNU df
	// output. On hosts where every filesystem name fits in fewer than
	// 14 chars (typical containers with sources
	// like /dev/vda, tmpfs, shm) the source column would otherwise
	// collapse to "Filesystem 1K-blocks ..." instead of GNU's padded
	// "Filesystem      1K-blocks ...". The Used minimum (5) keeps the
	// column from undercutting the header when the host has only
	// tiny filesystems.
	widths := minColumnWidths(withType)
	for _, row := range table {
		for i, cell := range row {
			widths[i] = max(widths[i], len(cell))
		}
	}

	// Filesystem (left-aligned) and Mounted on (left-aligned, no
	// trailing pad) frame the row; everything between is right-aligned.
	last := len(widths) - 1
	for _, row := range table {
		var b strings.Builder
		for i, cell := range row {
			if i > 0 {
				b.WriteByte(' ')
			}
			pad := widths[i] - len(cell)
			switch i {
			case 0:
				b.WriteString(cell)
				b.WriteString(strings.Repeat(" ", pad))
			case last:
				b.WriteString(cell)
			default:
				b.WriteString(strings.Repeat(" ", pad))
				b.WriteString(cell)
			}
		}
		b.WriteByte('\n')
		callCtx.Out(b.String())
	}
}

// minColumnWidths returns the per-column minimum widths matching GNU df's
// output (SOURCE=14, FSTYPE=4, SIZE/USED/AVAIL=5, USE%=4). Headers are
// always at least the label width, so the only minimums that exceed their
// header are SOURCE (14 vs "Filesystem"=10) and USED (5 vs "Used"=4).
func minColumnWidths(withType bool) []int {
	if withType {
		// Filesystem, Type, blocks, Used, Available, Use%, Mounted on
		return []int{14, 4, 5, 5, 5, 4, 0}
	}
	return []int{14, 5, 5, 5, 4, 0}
}

// printHelp emits the help text to stdout (per RULES.md, help is not an
// error; exit 0 with output on stdout).
//
// pflag.PrintDefaults handles every flag except -k, which is registered
// shorthand-only (GNU has no --kibibytes long form) and would otherwise
// render as the bogus "-k, --" line. -k is marked Hidden so it is
// skipped by PrintDefaults; we append the line manually so it still
// appears in --help.
//
// Every no-argument flag uses noArgSentinel (a NUL byte) as its
// NoOptDefVal so that explicit-value forms (--all=true, etc.) are
// rejected. PrintDefaults would happily render that NUL byte into the
// help text as `--all[= ]\x00…`, producing binary garbage. Clear the
// NoOptDefVal of every flag before printing — Parse has already run, so
// the cleared value cannot affect parsing for this invocation. The
// defer-restore is defensive: makeFlags currently builds a fresh
// FlagSet per invocation, but a future refactor that reuses or caches
// the FlagSet would silently regress the explicit-value rejection
// without it (--all=true would start being accepted).
func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: df [OPTION]...\n")
	callCtx.Out("Show information about the file system on which each FILE resides,\n")
	callCtx.Out("or all file systems by default.\n\n")
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
	callCtx.Out("  -k                               use 1024-byte blocks (POSIX default)\n")
}
