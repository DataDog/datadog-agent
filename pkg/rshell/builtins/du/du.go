// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package du implements the du builtin command.
//
// du — estimate file space usage
//
// Usage: du [OPTION]... [FILE]...
//
// Summarize device usage of the set of FILEs, recursively for directories.
// With no FILE, du operates on the current directory.
//
// Output format: "<size>\t<path>\n" per entry. Sizes are reported in
// 1024-byte blocks by default (this shell does not honour POSIXLY_CORRECT).
//
// Accepted flags:
//
//	-a, --all
//	    Write counts for all files, not just directories.
//
//	-s, --summarize
//	    Display only a per-argument total. Mutually exclusive with -a
//	    and with --max-depth.
//
//	-c, --total
//	    Produce a grand total row.
//
//	-d, --max-depth=N
//	    Print the total for a directory (or file, with --all) only if it
//	    is N or fewer levels below the command-line argument.
//	    --max-depth=0 is equivalent to --summarize.
//
//	-S, --separate-dirs
//	    For directories, do not include size of subdirectories.
//
//	-L, --dereference
//	    Follow all symbolic links during traversal. Cycles are detected
//	    via dev+inode identity and reported as errors.
//
//	-P, --no-dereference
//	    Never follow symbolic links (this is the default).
//
//	-0, --null
//	    End each output line with NUL, not newline.
//
//	-h, --human-readable
//	    Print sizes in human-readable format using 1024-power units
//	    (e.g. 1.0K, 234M, 2.0G).
//
//	--si
//	    Like -h, but use powers of 1000.
//
//	-k
//	    Use 1024-byte blocks (this is already the default).
//
//	-m
//	    Use 1 MiB (1024*1024) blocks.
//
//	-b, --bytes
//	    Equivalent to --apparent-size --block-size=1: report apparent
//	    size in bytes.
//
//	--apparent-size
//	    Print apparent sizes (file size in bytes) rather than allocated
//	    disk usage. Apparent sizes ignore sparse-file holes, internal
//	    fragmentation, and indirect blocks.
//
//	--help
//	    Print this usage message to stdout and exit 0.
//
// Rejected for security or scope:
//
//	--files0-from=FILE     Reads filenames from another file; data
//	                       exfiltration risk in sandboxed environments.
//	                       Same rationale as wc --files0-from.
//	--exclude-from=FILE    Reads exclude patterns from a file; same class.
//	-X, --exclude-from     (alias of --exclude-from)
//	--exclude=PATTERN      Glob-pattern filter; deferred until a safe
//	                       glob implementation lands. The shell's
//	                       glob→regex translator is currently sensitive
//	                       to certain multi-byte sequences (see fuzz
//	                       findings) and shipping --exclude before
//	                       hardening that path would expose pattern-
//	                       triggered failures to the du surface.
//
// All unknown flags are rejected by pflag with exit code 1, so
// security-sensitive flags above are simply not registered.
//
// Behaviour notes that intentionally diverge from GNU du:
//
//   - When `-P` is in effect (the default), a top-level operand that is itself
//     a symbolic link is reported as the symlink, not its target. GNU follows
//     the operand-level link in this case but our implementation prefers the
//     stricter no-follow-by-default reading. Use `-L` to follow.
//
// Exit codes:
//
//	0  All operands processed successfully.
//	1  At least one error occurred (missing file, permission denied,
//	   invalid argument, etc.).
//
// Memory and resource bounds:
//
//	Directory entries are read via callCtx.OpenDir's streaming
//	ReadDirFile so memory usage is proportional to traversal depth, not
//	directory width. Recursion is capped at maxRecursionDepth (256).
//	Each directory is opened in a per-iteration scope so its file
//	descriptor closes before recursion descends — depth × 1 FD instead
//	of depth × N. Hardlink-dedup tracking is bounded at maxDedupEntries
//	(1<<20) per call to prevent unbounded growth on adversarially
//	hardlink-rich subtrees; once the cap is hit, further hardlinks are
//	counted multiple times rather than triggering a memory exhaustion.
package du

import (
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"math"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the du builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "du",
	Description: "estimate file space usage",
	MakeFlags:   registerFlags,
}

// maxRecursionDepth caps recursion to prevent stack overflow from
// adversarially deep directory trees.
const maxRecursionDepth = 256

// statBlockUnit is the unit GNU du uses for the raw size derived from
// Stat_t.Blocks (always 512 regardless of the filesystem block size).
const statBlockUnit = 512

// apparentBlockSize is the rounding granularity for the apparent-size
// fallback used when the platform does not expose Stat_t.Blocks (e.g.
// Windows). 1024 matches the default GNU du block size.
const apparentBlockSize = 1024

// maxDedupEntries caps the hardlink-dedup tracking map to prevent unbounded
// memory growth when traversing pathological subtrees. Once exceeded,
// further hardlinks are counted as if they were independent files.
const maxDedupEntries = 1 << 20

// errFailed is a sentinel signaling that at least one entry failed.
var errFailed = errors.New("du: one or more errors occurred")

// unitMode selects how raw byte counts are formatted for output.
type unitMode int

const (
	unitKilo  unitMode = iota // 1024-byte blocks (default and -k)
	unitMega                  // 1 MiB blocks (-m)
	unitBytes                 // single bytes (-b / --bytes)
	unitHuman                 // human-readable, 1024-power (-h / --human-readable)
	unitSI                    // human-readable, 1000-power (--si)
)

type options struct {
	all          bool
	summarize    bool
	total        bool
	separateDirs bool
	dereference  bool // -L
	apparentSize bool
	null         bool
	maxDepth     int // -1 = unlimited
	maxDepthSet  bool
	unit         unitMode
}

// seqBool is a pflag.Value that records the sequence number of the most
// recent Set(true) call. Multiple invocations of the same flag
// (e.g. `-P -L -P`) each increment the shared counter, so the largest
// lastSet across a group of mutually-exclusive flags identifies the
// user's final choice.
//
// Set(false) — invoked via `--flag=false` — clears lastSet so the
// resolver does not pick this flag based on position alone.
//
// pflag.Visit only reports each flag once (at its first-set position),
// which loses repeated occurrences. seqBool is the workaround.
type seqBool struct {
	val     bool
	seq     *int // shared counter across all flags in this invocation
	lastSet int  // 0 = never set, or explicitly cleared via `--flag=false`
}

func (b *seqBool) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	b.val = v
	if v {
		*b.seq++
		b.lastSet = *b.seq
	} else {
		// Explicit `--flag=false` clears any prior position so the
		// resolver does not select this flag based solely on parse
		// order. Without this, `du --dereference=false link` would
		// still enable dereference-equivalent behaviour because
		// derefL.lastSet would be the highest in the group.
		b.lastSet = 0
	}
	return nil
}

func (b *seqBool) String() string   { return strconv.FormatBool(b.val) }
func (b *seqBool) Type() string     { return "bool" }
func (b *seqBool) IsBoolFlag() bool { return true }

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	// Preserve registration order so PrintDefaults emits flags in a stable
	// shape rather than alphabetical.
	fs.SortFlags = false

	all := fs.BoolP("all", "a", false, "write counts for all files, not just directories")
	summarize := fs.BoolP("summarize", "s", false, "display only a total for each argument")
	total := fs.BoolP("total", "c", false, "produce a grand total")
	separateDirs := fs.BoolP("separate-dirs", "S", false, "for directories, do not include size of subdirectories")

	// Mutually-exclusive last-wins groups (-L vs -P, and the size-format
	// flags -b/-h/--si/-k/-m). Each Set() call increments a shared
	// sequence counter, so the largest lastSet across the group identifies
	// the user's final choice — including repetitions like `du -P -L -P`
	// which pflag's Visit collapses to a single occurrence.
	//
	// Helper: register a custom Var-based bool flag with the parser-side
	// NoOptDefVal="true" trick that BoolP sets internally, so pflag treats
	// `-L`/`-P`/etc. as no-argument flags.
	seqCounter := new(int)
	registerSeq := func(name, shorthand, usage string) *seqBool {
		v := &seqBool{seq: seqCounter}
		f := fs.VarPF(v, name, shorthand, usage)
		f.NoOptDefVal = "true"
		return v
	}
	derefL := registerSeq("dereference", "L", "dereference all symbolic links")
	derefP := registerSeq("no-dereference", "P", "don't follow any symbolic links (default)")

	apparentSize := fs.Bool("apparent-size", false, "print apparent sizes rather than device usage")
	bytesFlag := registerSeq("bytes", "b", "equivalent to --apparent-size --block-size=1")
	humanFlag := registerSeq("human-readable", "h", "print sizes in human-readable format")
	siFlag := registerSeq("si", "", "like -h, but use powers of 1000")
	kiloFlag := registerSeq("kilobytes", "k", "use 1024-byte blocks (default)")
	megaFlag := registerSeq("megabytes", "m", "use 1 MiB (1024*1024) blocks")

	null := fs.BoolP("null", "0", false, "end each output line with NUL, not newline")
	maxDepth := fs.IntP("max-depth", "d", -1, "print the total for a directory only if it is N or fewer levels deep")
	helpFlag := fs.Bool("help", false, "print usage and exit")

	return func(ctx context.Context, callCtx *builtins.CallContext, paths []string) builtins.Result {
		if *helpFlag {
			fs.SetOutput(callCtx.Stdout)
			callCtx.Out("Usage: du [OPTION]... [FILE]...\n")
			callCtx.Out("Summarize device usage of the set of FILEs, recursively for directories.\n")
			callCtx.Out("With no FILE, du operates on the current directory.\n\n")
			fs.PrintDefaults()
			return builtins.Result{}
		}

		opts := options{
			all:          *all,
			summarize:    *summarize,
			total:        *total,
			separateDirs: *separateDirs,
			apparentSize: *apparentSize,
			null:         *null,
			maxDepth:     *maxDepth,
			maxDepthSet:  fs.Changed("max-depth"),
			unit:         unitKilo, // GNU default when no size-format flag is set
		}
		// Resolve `-L` vs `-P` last-wins by comparing sequence numbers.
		// Repeated invocations like `du -P -L -P` are honoured because each
		// Set() call updates lastSet on its respective seqBool.
		if derefL.lastSet > derefP.lastSet {
			opts.dereference = true
		} else if derefP.lastSet > derefL.lastSet {
			opts.dereference = false
		}
		// Resolve the size-format group (-b / -h / --si / -k / -m) the same
		// way: pick the flag with the highest lastSet sequence.
		sizeChoices := []struct {
			flag *seqBool
			unit unitMode
		}{
			{bytesFlag, unitBytes},
			{humanFlag, unitHuman},
			{siFlag, unitSI},
			{kiloFlag, unitKilo},
			{megaFlag, unitMega},
		}
		bestSeq := 0
		for _, c := range sizeChoices {
			if c.flag.lastSet > bestSeq {
				bestSeq = c.flag.lastSet
				opts.unit = c.unit
			}
		}
		// `-b` is shorthand for `--apparent-size --block-size=1`. Apparent
		// mode is sticky: once `-b` has appeared anywhere on the command
		// line, the totals remain apparent-size even if a later -k/-m
		// changed the unit. Matches GNU semantics for `du -b -k`.
		if bytesFlag.lastSet > 0 {
			opts.apparentSize = true
		}

		// Validate the raw --max-depth value first. This must precede the
		// `-s` normalisation below, which would otherwise overwrite a
		// negative -d argument with 0 and silently accept `du -s -d -1`.
		if opts.maxDepthSet && opts.maxDepth < 0 {
			callCtx.Errf("du: invalid maximum depth %d\n", opts.maxDepth)
			return builtins.Result{Code: 1}
		}

		// Mutual-exclusion checks (GNU semantics).
		// `-s` and `--max-depth=N` are equivalent at N=0; GNU prints a
		// warning for that case but exits 0. Any non-zero --max-depth
		// truly conflicts with -s and is a hard error.
		if opts.summarize && opts.maxDepthSet && opts.maxDepth > 0 {
			callCtx.Errf("du: summarizing conflicts with --max-depth=%d\n", opts.maxDepth)
			return builtins.Result{Code: 1}
		}
		if opts.summarize && opts.maxDepthSet && opts.maxDepth == 0 {
			callCtx.Errf("du: warning: summarizing is the same as using --max-depth=0\n")
		}
		if opts.summarize && opts.all {
			callCtx.Errf("du: cannot both summarize and show all entries\n")
			return builtins.Result{Code: 1}
		}
		if opts.summarize {
			opts.maxDepth = 0
			opts.maxDepthSet = true
		}

		if len(paths) == 0 {
			paths = []string{"."}
		}

		// Hardlink dedup: count each (dev,inode) only once across the run.
		// Bounded at maxDedupEntries to prevent unbounded growth.
		visited := map[builtins.FileID]bool{}
		var grandTotal int64
		failed := false

		for _, p := range paths {
			if ctx.Err() != nil {
				break
			}
			size, _, err := walk(ctx, callCtx, p, p, 0, opts, visited, nil)
			if err != nil {
				failed = true
			}
			grandTotal = saturatingAdd(grandTotal, size)
		}

		if opts.total {
			emit(callCtx, opts, grandTotal, "total")
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

// walk processes a single operand or recursive entry. It returns:
//   - size: the subtree size to attribute to this entry. Under
//     --separate-dirs this excludes any subdirectory subtree; otherwise
//     it is the full recursive total.
//   - isDir: whether the entry was treated as a directory (false for
//     symlinks under -P, true for symlinks-to-dirs under -L). The parent
//     uses this to decide whether to skip this child under
//     --separate-dirs.
//   - err: non-nil if the entry could not be processed.
//
// reportPath is the path as written on the command line (for output).
// fsPath is the actual path to read (same as reportPath for top-level
// operands; joined paths during recursion).
// depth is 0 for the operand itself, 1 for its children, etc.
// ancestorIDs tracks visited directory identities along the recursion stack
// for symlink-loop detection in -L mode.
func walk(
	ctx context.Context,
	callCtx *builtins.CallContext,
	fsPath string,
	reportPath string,
	depth int,
	opts options,
	visited map[builtins.FileID]bool,
	ancestorIDs map[builtins.FileID]string,
) (size int64, isDir bool, err error) {
	if ctx.Err() != nil {
		return 0, false, ctx.Err()
	}
	if depth > maxRecursionDepth {
		callCtx.Errf("du: recursion depth limit exceeded at '%s'\n", builtins.SafeOperand(reportPath))
		return 0, false, errFailed
	}

	info, err := statEntry(ctx, callCtx, fsPath, opts.dereference)
	if err != nil {
		callCtx.Errf("du: cannot access '%s': %s\n", builtins.SafeOperand(reportPath), callCtx.PortableErr(err))
		return 0, false, err
	}

	// Regular-file dedup. GNU du suppresses any inode it has already
	// counted in the same invocation (the opposite of `--count-links`,
	// which we don't implement). This covers hard-linked files visited
	// via different paths and regular files reached via two `-L`
	// symlinks. Non-regular non-dir leaves (symlinks under -P, device
	// nodes, FIFOs) are not deduped.
	if info.Mode().IsRegular() && callCtx.FileIdentity != nil {
		if id, ok := callCtx.FileIdentity(fsPath, info); ok {
			if visited[id] {
				return 0, false, nil
			}
			if len(visited) < maxDedupEntries {
				visited[id] = true
			}
		}
	}

	// Non-directory leaf (regular file, symlink under -P, dangling link).
	// Always reports its own size; --separate-dirs does not exclude file
	// children — only subdirectory subtrees.
	if !info.IsDir() {
		fileSize := entrySize(info, opts.apparentSize)
		if shouldEmit(depth, false, opts) {
			emit(callCtx, opts, fileSize, reportPath)
		}
		return fileSize, false, nil
	}

	// Directory dedup + cycle detection. Cycle detection must precede
	// dedup so that a symlink loop reported via `du -L` still surfaces
	// the loop error rather than silently terminating at "we've already
	// visited this inode". Both checks key off the same FileID.
	if callCtx.FileIdentity != nil {
		if id, ok := callCtx.FileIdentity(fsPath, info); ok {
			// Cycle: the directory is on the current recursion stack.
			if opts.dereference {
				if firstPath, seen := ancestorIDs[id]; seen {
					callCtx.Errf("du: File system loop detected; '%s' is part of the same file system loop as '%s'.\n",
						builtins.SafeOperand(reportPath), builtins.SafeOperand(firstPath))
					return 0, true, errFailed
				}
				// Push this directory onto the ancestor map for the duration
				// of the recursion below; pop on the way back up via defer.
				// This avoids an O(depth²) clone per level — the map is
				// shared across the whole recursion tree.
				ancestorIDs = pushAncestor(ancestorIDs, id, reportPath)
				defer delete(ancestorIDs, id)
			}
			// Dedup: the directory has already been walked in a different
			// branch of this invocation (e.g. `du d d` or two -L symlinks
			// pointing at the same dir). GNU suppresses the second
			// occurrence; we do the same.
			if visited[id] {
				return 0, true, nil
			}
			if len(visited) < maxDedupEntries {
				visited[id] = true
			}
		}
	}

	dirOwn := entrySize(info, opts.apparentSize)
	fileChildren, subdirSubtrees, failedAny := walkChildren(ctx, callCtx, fsPath, reportPath, depth, opts, visited, ancestorIDs)

	// fullSubtree is the recursive total: own bytes + direct files +
	// every subdirectory's full subtree. This is what gets returned to
	// the parent and ultimately summed into the grand total under -c.
	fullSubtree := saturatingAdd(saturatingAdd(dirOwn, fileChildren), subdirSubtrees)

	// printedSize is what gets emitted to stdout. Under --separate-dirs
	// it excludes subdirectory subtrees; otherwise it equals fullSubtree.
	printedSize := fullSubtree
	if opts.separateDirs {
		printedSize = saturatingAdd(dirOwn, fileChildren)
	}
	if shouldEmit(depth, true, opts) {
		emit(callCtx, opts, printedSize, reportPath)
	}

	// Always return the full subtree so a `-c` grand total or a parent
	// without `-S` sees the complete recursive value. GNU's grand total
	// is the sum of operand subtrees regardless of `-S`, so returning
	// the printed value (with subdirs stripped) here would underreport.
	if failedAny {
		return fullSubtree, true, errFailed
	}
	return fullSubtree, true, nil
}

// walkChildren iterates entries in dir via OpenDir/ReadDir(1), recursing
// into walk for each. Scoped as a separate function so the directory
// handle's defer Close() fires at this frame's exit rather than the
// outer walk's, keeping FD usage proportional to depth × 1 not depth × N.
//
// Returns the file-children sum and the subdirectory-children sum
// separately so that the caller can apply --separate-dirs (which
// excludes only subdirectory contributions, not direct file children).
func walkChildren(
	ctx context.Context,
	callCtx *builtins.CallContext,
	fsPath string,
	reportPath string,
	depth int,
	opts options,
	visited map[builtins.FileID]bool,
	ancestorIDs map[builtins.FileID]string,
) (fileChildren, subdirChildren int64, failedAny bool) {
	dh, err := callCtx.OpenDir(ctx, fsPath)
	if err != nil {
		callCtx.Errf("du: cannot read directory '%s': %s\n", builtins.SafeOperand(reportPath), callCtx.PortableErr(err))
		return 0, 0, true
	}
	defer dh.Close()

	for {
		if ctx.Err() != nil {
			return fileChildren, subdirChildren, true
		}
		entries, readErr := dh.ReadDir(1)
		if len(entries) == 0 {
			if readErr == nil || errors.Is(readErr, io.EOF) {
				return fileChildren, subdirChildren, failedAny
			}
			callCtx.Errf("du: error reading directory '%s': %s\n", builtins.SafeOperand(reportPath), callCtx.PortableErr(readErr))
			return fileChildren, subdirChildren, true
		}
		ent := entries[0]
		childFs := joinPath(fsPath, ent.Name())
		childReport := joinPath(reportPath, ent.Name())
		childSize, childIsDir, walkErr := walk(ctx, callCtx, childFs, childReport, depth+1, opts, visited, ancestorIDs)
		if walkErr != nil {
			failedAny = true
		}
		if childIsDir {
			subdirChildren = saturatingAdd(subdirChildren, childSize)
		} else {
			fileChildren = saturatingAdd(fileChildren, childSize)
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			callCtx.Errf("du: error reading directory '%s': %s\n", builtins.SafeOperand(reportPath), callCtx.PortableErr(readErr))
			return fileChildren, subdirChildren, true
		}
	}
}

// pushAncestor inserts (id, path) into ancestorIDs (allocating a new map
// on first push) and returns the same map. The caller is expected to
// `defer delete(m, id)` to pop the entry when its recursion frame exits.
func pushAncestor(m map[builtins.FileID]string, id builtins.FileID, path string) map[builtins.FileID]string {
	if m == nil {
		m = make(map[builtins.FileID]string, 4)
	}
	m[id] = path
	return m
}

// shouldEmit reports whether an entry at the given depth should be printed
// under the active options.
//
// Files (non-dirs) print only with -a or when the file is a top-level
// operand. With -s only depth 0 prints. --max-depth caps the printable
// depth without affecting accumulation.
func shouldEmit(depth int, isDir bool, opts options) bool {
	if opts.summarize {
		return depth == 0
	}
	if opts.maxDepthSet && depth > opts.maxDepth {
		return false
	}
	if !isDir && depth > 0 && !opts.all {
		return false
	}
	return true
}

// entrySize returns the raw byte count attributed to an entry.
//
// Behaviour matches GNU du across platforms:
//
//   - Non-directory files in apparent-size mode use info.Size().
//
//   - Non-directory files in disk-usage mode use Stat_t.Blocks * 512, or
//     (when Blocks is unavailable) info.Size() rounded up to the nearest
//     1024-byte block.
//
//   - Directories use Stat_t.Blocks * 512 in *both* modes. The GNU
//     manual's wording about apparent-size being "for regular files and
//     symbolic links" describes the *typical* outcome — not a special
//     case in the code path. GNU treats every entry uniformly:
//     apparent_size mode uses info.Size() for non-dirs and Blocks*512
//     for dirs (where Blocks captures whatever the filesystem reports).
//     This produces filesystem-dependent dir totals, which is GNU's
//     actual observable behaviour:
//
//     | Filesystem | dir Blocks | du -b empty/ | du -b dir/ (3-byte child) |
//     |------------|-----------:|-------------:|---------------------------:|
//     | macOS APFS |          0 |            0 |                          3 |
//     | Linux ext4 |          8 |         4096 |                       4099 |
//     | Linux tmp  |          0 |            0 |                          3 |
//     | Windows    |        n/a |            0 |                          3 |
//
//     Verified against `du (GNU coreutils) 9.1` on debian:bookworm-slim:
//
//     $ mkdir d; printf abc > d/file
//     $ du -b d
//     4099	d
//
//     This means rshell's `du -b dir` matches GNU on the host's
//     filesystem rather than producing a normalized cross-platform
//     value — same trade-off GNU itself makes.
//
// The Blocks * 512 multiplication is clamped to math.MaxInt64 to defend
// against pathological filesystems (e.g. FUSE) that report bogus values.
func entrySize(info iofs.FileInfo, apparent bool) int64 {
	if info.IsDir() {
		if blocks, ok := infoBlocks(info); ok {
			return clampMul(blocks, statBlockUnit)
		}
		return 0
	}
	if apparent {
		return info.Size()
	}
	if blocks, ok := infoBlocks(info); ok {
		return clampMul(blocks, statBlockUnit)
	}
	size := info.Size()
	if size <= 0 {
		return 0
	}
	if size > math.MaxInt64-apparentBlockSize+1 {
		return math.MaxInt64
	}
	return ((size + apparentBlockSize - 1) / apparentBlockSize) * apparentBlockSize
}

// clampMul multiplies a*b for non-negative inputs, returning math.MaxInt64
// on overflow and 0 on negative inputs. This guards against pathological
// Stat_t.Blocks values from untrusted filesystems.
func clampMul(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	if a > math.MaxInt64/b {
		return math.MaxInt64
	}
	return a * b
}

// saturatingAdd returns a+b, clamped to math.MaxInt64 to avoid wraparound
// when accumulating sizes across enormous subtrees.
func saturatingAdd(a, b int64) int64 {
	if a < 0 {
		a = 0
	}
	if b < 0 {
		b = 0
	}
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// formatSize converts a raw byte count into the unit configured by opts.
// Block units round up (matching GNU); human and SI variants pick the
// smallest unit ≥ base.
func formatSize(rawBytes int64, opts options) string {
	switch opts.unit {
	case unitBytes:
		return strconv.FormatInt(rawBytes, 10)
	case unitMega:
		return strconv.FormatInt(divCeil(rawBytes, 1024*1024), 10)
	case unitHuman:
		return humanSize(rawBytes, 1024, []string{"B", "K", "M", "G", "T", "P", "E"})
	case unitSI:
		return humanSize(rawBytes, 1000, []string{"B", "k", "M", "G", "T", "P", "E"})
	case unitKilo:
		fallthrough
	default:
		return strconv.FormatInt(divCeil(rawBytes, 1024), 10)
	}
}

// divCeil performs integer ceiling division for non-negative inputs.
// Negative or zero inputs return 0.
func divCeil(n, d int64) int64 {
	if n <= 0 {
		return 0
	}
	if n > math.MaxInt64-d+1 {
		// Saturate rather than wrap: the value is already at the limit.
		return math.MaxInt64 / d
	}
	return (n + d - 1) / d
}

// humanSize formats a byte count using the supplied base (1024 or 1000).
// Below the base it prints the raw integer with no suffix (matching GNU).
// At base or above it picks the smallest unit such that value < base,
// printing one decimal when the value is < 10 in that unit (so "1.5K"
// but "234M") and zero decimals otherwise.
//
// GNU `du -h` rounds *up* at the displayed precision rather than to
// nearest, so 1025 bytes prints "1.1K" (not "1.0K") and 10241 bytes
// prints "11K" (not "10K"). We replicate this with explicit ceiling
// rounding before formatting.
func humanSize(rawBytes int64, base int64, units []string) string {
	if rawBytes < 0 {
		rawBytes = 0
	}
	if rawBytes < base {
		return strconv.FormatInt(rawBytes, 10)
	}
	val := float64(rawBytes)
	div := float64(base)
	for i := 1; i < len(units); i++ {
		val /= div
		if val < float64(base) {
			// Decide one-decimal vs zero-decimal display based on the
			// rounded-up value, not the raw float, so e.g. 9.95 rounds
			// up to 10 (no decimal) but 9.94 stays at "9.9".
			oneDecCeil := math.Ceil(val*10) / 10
			if oneDecCeil < 10 {
				return fmt.Sprintf("%.1f%s", oneDecCeil, units[i])
			}
			return fmt.Sprintf("%.0f%s", math.Ceil(val), units[i])
		}
	}
	return fmt.Sprintf("%.0f%s", math.Ceil(val), units[len(units)-1])
}

// emit writes a single output line: "<size>\t<path>" terminated by \n
// (or \x00 with --null).
func emit(callCtx *builtins.CallContext, opts options, rawBytes int64, path string) {
	terminator := "\n"
	if opts.null {
		terminator = "\x00"
	}
	callCtx.Outf("%s\t%s%s", formatSize(rawBytes, opts), path, terminator)
}

// statEntry stats a path, following symlinks when -L is set.
//
// Note: this function does NOT follow operand-level symlinks even at
// depth 0 unless -L is supplied — see the package-level "Behaviour notes"
// for the GNU divergence.
func statEntry(ctx context.Context, callCtx *builtins.CallContext, path string, deref bool) (iofs.FileInfo, error) {
	if deref {
		return callCtx.StatFile(ctx, path)
	}
	return callCtx.LstatFile(ctx, path)
}

// joinPath joins a directory and a name without invoking filepath.Clean,
// preserving '.' and '..' segments so that operand-relative paths are
// reported the same way GNU du reports them. This intentionally matches
// the helper at builtins/find/find.go:645 — paths are canonicalised by
// the sandbox at lookup time, but reported verbatim to the user.
func joinPath(dir, name string) string {
	if len(dir) == 0 {
		return name
	}
	if dir[len(dir)-1] == '/' {
		return dir + name
	}
	return dir + "/" + name
}
