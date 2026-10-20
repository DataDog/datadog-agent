// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ls implements the ls builtin command.
//
// ls — list directory contents
//
// Usage: ls [OPTION]... [FILE]...
//
// List information about the FILEs (the current directory by default).
// Sort entries alphabetically by default.
//
// Accepted flags:
//
//	-1
//	    List one file per line. This is the default when output is not a
//	    terminal (which is always the case in this shell).
//
//	-a, --all
//	    Do not ignore entries starting with . (includes . and ..).
//
//	-A, --almost-all
//	    Do not ignore entries starting with . but omit . and ..
//
//	-d, --directory
//	    List directories themselves, not their contents.
//
//	-r, --reverse
//	    Reverse order while sorting.
//
//	-S
//	    Sort by file size, largest first.
//
//	-t
//	    Sort by modification time, newest first.
//
//	-F, --classify
//	    Append indicator (one of * / = @ |) to entries.
//
//	-p
//	    Append / indicator to directories.
//
//	-R, --recursive
//	    List subdirectories recursively.
//
//	-l
//	    Use a long listing format: mode, hard links, owner, group, size, date, name.
//
//	-h, --human-readable
//	    With -l, print sizes in human-readable format (e.g. 1K, 234M).
//
//	--offset N  (non-standard)
//	    Skip the first N raw directory entries before collecting results.
//	    NOTE: offset operates on filesystem order, not sorted order —
//	    entries are sorted only within each page. For predictable
//	    pagination of large directories, use consistent offset/limit
//	    pairs. Applies to single-directory listings only; silently
//	    ignored with -R or multiple arguments.
//
//	--limit N  (non-standard)
//	    Show at most N entries, capped at MaxDirEntries (1,000) per call.
//	    Applies to single-directory listings only; silently ignored with -R
//	    or multiple arguments.
//
// When a directory exceeds MaxDirEntries (1,000) and no explicit --offset/
// --limit is given, ls prints the first 1,000 entries, emits a warning to
// stderr, and exits 1. Use --offset/--limit to paginate through larger
// directories.
//
// Exit codes:
//
//	0  All entries listed successfully.
//	1  At least one error occurred (missing file, permission denied, etc.).
//	1  Directory truncated (exceeded MaxDirEntries without --offset/--limit).
//	1  Invalid usage (unrecognised flag, etc.).
package ls

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"runtime"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// maxRecursionDepth limits how deep -R will recurse to prevent stack overflow.
const maxRecursionDepth = 256

// MaxDirEntries is the maximum number of entries ls will process per directory.
const MaxDirEntries = 1_000

// errFailed is a sentinel used to signal that at least one entry had an error.
var errFailed = errors.New("ls: one or more errors occurred")

// Cmd is the ls builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "ls",
	Description: "list directory contents",
	MakeFlags:   registerFlags,
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	// Preserve parse order so Visit() returns flags in the order set.
	// This lets us determine which sort flag (-S or -t) was specified last.
	fs.SortFlags = false

	_ = fs.BoolP("all", "a", false, "do not ignore entries starting with .")
	_ = fs.BoolP("almost-all", "A", false, "do not ignore . and ..")
	dirOnly := fs.BoolP("directory", "d", false, "list directories themselves, not their contents")
	reverse := fs.BoolP("reverse", "r", false, "reverse order while sorting")
	_ = fs.BoolP("sort-size", "S", false, "sort by file size, largest first")
	_ = fs.BoolP("sort-time", "t", false, "sort by modification time, newest first")
	classify := fs.BoolP("classify", "F", false, "append indicator to entries")
	appendSlash := fs.BoolP("append-slash", "p", false, "append / indicator to directories")
	recursive := fs.BoolP("recursive", "R", false, "list subdirectories recursively")
	longFmt := fs.BoolP("long", "l", false, "use a long listing format")
	humanReadable := fs.BoolP("human-readable", "h", false, "with -l, print human-readable sizes")
	// -1 is the default in non-terminal (always true here), accepted for compat.
	_ = fs.Bool("one", false, "list one file per line")
	fs.Lookup("one").Shorthand = "1"
	// Pagination flags (long-only, non-standard).
	offset := fs.Int("offset", 0, "skip first N entries (pagination)")
	limit := fs.Int("limit", 0, "show at most N entries (capped at MaxDirEntries)")

	// Help flag (long-only; -h is taken by --human-readable).
	help := fs.Bool("help", false, "print usage and exit")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			callCtx.Out("Usage: ls [OPTION]... [FILE]...\n")
			callCtx.Out("List directory contents.\n")
			callCtx.Out("List information about the FILEs (the current directory by default).\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		now := callCtx.Now

		// Determine the effective sort mode. When both -S and -t are given,
		// the last one specified wins, matching GNU ls behaviour.
		//
		// Determine the effective hidden-file mode. When both -a and -A are
		// given, the last one specified wins, matching GNU ls behaviour.
		//
		// NOTE: fs.Visit iterates flags in parse-order (SortFlags=false) but
		// only visits each flag once, so repeated flags like -a -A -a cannot
		// be fully resolved. This is an acceptable limitation for such a rare
		// edge case.
		var sortSize, sortTime bool
		var showAll, showAlmostAll bool
		fs.Visit(func(f *builtins.Flag) {
			switch f.Name {
			case "sort-size":
				sortSize = true
				sortTime = false
			case "sort-time":
				sortTime = true
				sortSize = false
			case "all":
				showAll = true
				showAlmostAll = false
			case "almost-all":
				showAlmostAll = true
				showAll = false
			}
		})

		opts := &options{
			all:           showAll,
			almostAll:     showAlmostAll,
			dirOnly:       *dirOnly,
			reverse:       *reverse,
			sortSize:      sortSize,
			sortTime:      sortTime,
			classify:      *classify,
			appendSlash:   *appendSlash,
			recursive:     *recursive,
			longFmt:       *longFmt,
			humanReadable: *humanReadable,
			offset:        *offset,
			limit:         *limit,
		}

		paths := args
		if len(paths) == 0 {
			paths = []string{"."}
		}

		failed := false
		multipleArgs := len(paths) > 1

		// Pagination applies only to single-directory listings.
		if multipleArgs {
			opts.offset = 0
			opts.limit = 0
		}

		// Separate files and dirs (when not -d).
		// Use Lstat so that symlink operands retain ModeSymlink (GNU ls default).
		// The link target is only stat'd when needed (e.g. to decide dir vs file
		// for a symlink-to-directory, since -d is the only case where we'd list
		// the symlink itself rather than the target directory contents).
		var files []pathArg
		var dirs []pathArg
		for _, p := range paths {
			if ctx.Err() != nil {
				break
			}
			if p == "" {
				callCtx.Errf("ls: cannot access '': No such file or directory\n")
				failed = true
				continue
			}
			info, err := callCtx.LstatFile(ctx, p)
			if err != nil {
				callCtx.Errf("ls: cannot access '%s': %s\n", builtins.SafeOperand(p), callCtx.PortableErr(err))
				failed = true
				continue
			}
			// For symlinks to directories: follow the link to list contents
			// (unless -d is set, which lists the entry itself).
			isDir := info.IsDir()
			if info.Mode()&iofs.ModeSymlink != 0 && !opts.dirOnly {
				if target, err := callCtx.StatFile(ctx, p); err == nil {
					isDir = target.IsDir()
				}
			}
			if !isDir || opts.dirOnly {
				pa := pathArg{name: p, info: info}
				if opts.longFmt && info.Mode()&iofs.ModeSymlink != 0 {
					if t, err := callCtx.ReadlinkFile(ctx, p); err == nil {
						pa.linkTarget = t
					}
					// Stat the target for -F/-p indicators (nil for dangling links).
					if ti, err := callCtx.StatFile(ctx, p); err == nil {
						pa.linkTargetInfo = ti
					}
				}
				files = append(files, pa)
			} else {
				dirs = append(dirs, pathArg{name: p, info: info})
			}
		}

		// List individual files first.
		if len(files) > 0 {
			sortEntries(files, opts, func(a pathArg) iofs.FileInfo { return a.info }, func(a pathArg) string { return a.name })
			var cw colWidths
			if opts.longFmt {
				for i := range files {
					f := &files[i]
					f.owner, f.group, f.nlink = fileOwner(ctx, callCtx, f.name, f.info)
				}
				cw = computeColWidths(files, func(a pathArg) iofs.FileInfo { return a.info }, func(a pathArg) (string, string, string) { return a.owner, a.group, a.nlink }, opts)
			}
			for _, f := range files {
				printEntry(callCtx, f.name, f.info, f.owner, f.group, f.nlink, f.linkTarget, f.linkTargetInfo, opts, now, cw)
			}
		}

		// Sort and list directories.
		sortEntries(dirs, opts, func(a pathArg) iofs.FileInfo { return a.info }, func(a pathArg) string { return a.name })
		showHeader := multipleArgs || len(files) > 0 || opts.recursive
		for i, d := range dirs {
			if ctx.Err() != nil {
				break
			}
			if showHeader {
				if i > 0 || len(files) > 0 {
					callCtx.Out("\n")
				}
				callCtx.Outf("%s:\n", d.name)
			}
			if err := listDir(ctx, callCtx, d.name, opts, 0, now); err != nil {
				failed = true
			}
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

type options struct {
	all           bool
	almostAll     bool
	dirOnly       bool
	reverse       bool
	sortSize      bool
	sortTime      bool
	classify      bool
	appendSlash   bool
	recursive     bool
	longFmt       bool
	humanReadable bool
	offset        int
	limit         int
}

type pathArg struct {
	name           string
	info           iofs.FileInfo
	owner          string // cached by fileOwner (populated when longFmt)
	group          string
	nlink          string
	linkTarget     string        // symlink destination (populated when longFmt + symlink)
	linkTargetInfo iofs.FileInfo // target's FileInfo for indicator (may be nil for dangling links)
}

func listDir(ctx context.Context, callCtx *builtins.CallContext, dir string, opts *options, depth int, now time.Time) error {
	if depth > maxRecursionDepth {
		callCtx.Errf("ls: recursion depth limit exceeded at '%s'\n", builtins.SafeOperand(dir))
		return errFailed
	}

	// Clamp negative values to zero so they become no-ops.
	if opts.offset < 0 {
		opts.offset = 0
	}
	if opts.limit < 0 {
		opts.limit = 0
	}

	// Pagination applies only to single-directory, non-recursive listings.
	// Silently ignore --offset/--limit when used with -R.
	paginationActive := !opts.recursive && (opts.offset > 0 || opts.limit > 0)

	// Determine effective read limit.
	effectiveLimit := MaxDirEntries
	if opts.limit > 0 && !opts.recursive {
		effectiveLimit = min(opts.limit, MaxDirEntries)
	}

	// NOTE: MaxDirEntries caps raw directory reads (before hidden-file filtering)
	// intentionally. Applying the cap after filtering would allow DoS via
	// directories with many dotfiles — the shell would have to read unbounded
	// entries to find visible ones.
	//
	// When pagination is active, offset is passed to readDir so skipping happens
	// at the read level (O(n) memory regardless of offset). Otherwise offset=0.
	readOffset := 0
	if paginationActive {
		readOffset = opts.offset
	}
	entries, truncated, err := readDir(ctx, callCtx, dir, readOffset, effectiveLimit)
	if err != nil {
		callCtx.Errf("ls: cannot open directory '%s': %s\n", builtins.SafeOperand(dir), callCtx.PortableErr(err))
		return err
	}

	// Get FileInfo for sorting (if needed) and for long format.
	type entryInfo struct {
		name           string
		info           iofs.FileInfo
		isSymlink      bool
		owner          string // cached by fileOwner (populated when longFmt)
		group          string
		nlink          string
		linkTarget     string        // symlink destination (populated when longFmt + symlink)
		linkTargetInfo iofs.FileInfo // target's FileInfo for indicator (may be nil for dangling links)
	}

	failed := false
	var infoEntries []entryInfo

	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		name := e.Name()
		if len(name) > 0 && name[0] == '.' && !opts.all && !opts.almostAll {
			continue
		}
		info, infoErr := e.Info()
		if infoErr != nil {
			callCtx.Errf("ls: cannot access '%s': %s\n", builtins.SafeOperand(joinPath(dir, name)), callCtx.PortableErr(infoErr))
			failed = true
			continue
		}
		ei := entryInfo{
			name: name,
			info: info,
			// Check both Type() and info.Mode() — Type() can be 0 on
			// filesystems that report DT_UNKNOWN.
			isSymlink: e.Type()&iofs.ModeSymlink != 0 || info.Mode()&iofs.ModeSymlink != 0,
		}
		if opts.longFmt && ei.isSymlink {
			fullPath := joinPath(dir, name)
			if t, err := callCtx.ReadlinkFile(ctx, fullPath); err == nil {
				ei.linkTarget = t
			}
			// Stat the target for -F/-p indicators (nil for dangling links).
			if ti, err := callCtx.StatFile(ctx, fullPath); err == nil {
				ei.linkTargetInfo = ti
			}
		}
		infoEntries = append(infoEntries, ei)
	}

	// Synthesize . and .. for -a (os.ReadDir never includes them).
	// Added before sorting so they participate in sort modifiers (-r, -S, -t),
	// matching bash behavior. They do not consume offset/limit slots because
	// pagination is handled at the read level.
	if opts.all {
		if dotInfo, err := callCtx.StatFile(ctx, dir); err == nil {
			infoEntries = append(infoEntries, entryInfo{name: ".", info: dotInfo})
			// Try to stat the real parent so .. has correct metadata (size,
			// blocks, etc.). Fall back to . info if the parent is outside
			// the sandbox.
			dotdotInfo := dotInfo
			if parentInfo, err := callCtx.StatFile(ctx, joinPath(dir, "..")); err == nil {
				dotdotInfo = parentInfo
			}
			infoEntries = append(infoEntries, entryInfo{name: "..", info: dotdotInfo})
		}
	}

	// Sort all entries (including . and ..) so sort modifiers apply uniformly.
	sortEntries(infoEntries, opts, func(a entryInfo) iofs.FileInfo { return a.info }, func(a entryInfo) string { return a.name })

	// Offset is handled at the read level (streaming skip in readDir),
	// so no post-sort slicing is needed. The effectiveLimit is already
	// enforced by readDir's maxRead parameter.

	// Print.
	var cw colWidths
	if opts.longFmt {
		// Populate owner metadata once per entry (avoids double file opens on Windows).
		for i := range infoEntries {
			ei := &infoEntries[i]
			ei.owner, ei.group, ei.nlink = fileOwner(ctx, callCtx, joinPath(dir, ei.name), ei.info)
		}
		cw = computeColWidths(infoEntries, func(e entryInfo) iofs.FileInfo { return e.info }, func(e entryInfo) (string, string, string) { return e.owner, e.group, e.nlink }, opts)
		var totalBlocks int64
		blocksAvailable := true
		for _, ei := range infoEntries {
			b, ok := fileBlocks(ctx, callCtx, joinPath(dir, ei.name), ei.info)
			if !ok {
				blocksAvailable = false
				break
			}
			totalBlocks += b
		}
		if blocksAvailable {
			// fileBlocks returns 512-byte units; display in 1024-byte blocks.
			callCtx.Outf("total %d\n", totalBlocks/2)
		}
	}
	for _, ei := range infoEntries {
		if ctx.Err() != nil {
			break
		}
		printEntry(callCtx, ei.name, ei.info, ei.owner, ei.group, ei.nlink, ei.linkTarget, ei.linkTargetInfo, opts, now, cw)
	}

	// Only warn on implicit truncation (no explicit --offset/--limit).
	// Return immediately — do not recurse (-R) into subdirectories after
	// hitting the MaxDirEntries cap. This intentionally diverges from bash,
	// which would continue recursion. The cap exists to bound total work;
	// allowing recursion after truncation would let an adversarial tree
	// (e.g. 1000 subdirs × 1000 subdirs × ...) trigger unbounded traversal.
	if truncated && !paginationActive {
		callCtx.Errf("ls: warning: directory '%s': too many entries (exceeded %d limit), output truncated\n", builtins.SafeOperand(dir), MaxDirEntries)
		return errFailed
	}

	// Recurse into subdirectories if -R.
	// Symlinks are skipped to match GNU ls default behaviour (which does
	// not follow symlinks during -R traversal) and to prevent symlink-loop
	// denial-of-service attacks. The maxRecursionDepth limit provides an
	// additional safety bound.
	if opts.recursive {
		for _, ei := range infoEntries {
			if ctx.Err() != nil {
				break
			}
			if ei.isSymlink || !ei.info.IsDir() {
				continue
			}
			if ei.name == "." || ei.name == ".." {
				continue
			}
			subdir := joinPath(dir, ei.name)
			callCtx.Outf("\n%s:\n", subdir)
			if err := listDir(ctx, callCtx, subdir, opts, depth+1, now); err != nil {
				failed = true
			}
		}
	}

	if failed {
		return errFailed
	}
	return nil
}

// readDir dispatches to ReadDirLimited when available, falling back to ReadDir.
// The offset parameter skips raw directory entries at the read level (before sorting).
func readDir(ctx context.Context, callCtx *builtins.CallContext, dir string, offset, maxRead int) (entries []iofs.DirEntry, truncated bool, err error) {
	if callCtx.ReadDirLimited != nil {
		return callCtx.ReadDirLimited(ctx, dir, offset, maxRead)
	}
	entries, err = callCtx.ReadDir(ctx, dir)
	return entries, false, err
}

// colWidths holds the computed column widths for long-format output.
type colWidths struct {
	nlink int
	owner int
	group int
	size  int
}

// computeColWidths computes the maximum column widths across a slice of entries
// for long-format output alignment. Owner metadata must be pre-populated in
// the entries; getOwnerInfo extracts the cached (owner, group, nlink) strings.
func computeColWidths[T any](entries []T, getInfo func(T) iofs.FileInfo, getOwnerInfo func(T) (string, string, string), opts *options) colWidths {
	var w colWidths
	for _, e := range entries {
		owner, group, nlink := getOwnerInfo(e)

		if n := len(nlink); n > w.nlink {
			w.nlink = n
		}
		if n := len(owner); n > w.owner {
			w.owner = n
		}
		if n := len(group); n > w.group {
			w.group = n
		}

		var sizeStr string
		if opts.humanReadable {
			sizeStr = humanSize(getInfo(e).Size())
		} else {
			sizeStr = fmt.Sprintf("%d", getInfo(e).Size())
		}
		if n := len(sizeStr); n > w.size {
			w.size = n
		}
	}
	return w
}

func printEntry(callCtx *builtins.CallContext, name string, info iofs.FileInfo, owner, group, nlink, linkTarget string, linkTargetInfo iofs.FileInfo, opts *options, now time.Time, cw colWidths) {
	if opts.longFmt {
		mode := formatMode(info)
		size := info.Size()
		modTime := info.ModTime()

		var sizeStr string
		if opts.humanReadable {
			sizeStr = humanSize(size)
		} else {
			sizeStr = fmt.Sprintf("%d", size)
		}

		suffix := indicator(info, opts)
		if linkTarget != "" {
			targetIndicator := ""
			// GNU ls only applies -F (classify) indicators to symlink
			// targets, not -p (append-slash-only). For example,
			// ls -lF shows "link -> dir/" but ls -lp shows "link -> dir".
			if linkTargetInfo != nil && opts.classify {
				targetIndicator = indicator(linkTargetInfo, opts)
			}
			suffix = " -> " + linkTarget + targetIndicator
		}

		timeStr := formatTime(modTime, now)
		callCtx.Outf("%s %*s %-*s %-*s %*s %s %s%s\n",
			mode, cw.nlink, nlink,
			cw.owner, owner, cw.group, group,
			cw.size, sizeStr, timeStr,
			name, suffix)
	} else {
		callCtx.Outf("%s%s\n", name, indicator(info, opts))
	}
}

// formatMode returns a 10-character mode string matching GNU ls format.
// Go's FileMode.String() uses 'L' for symlinks (ls uses 'l') and doesn't
// render setuid/setgid/sticky in the rwx triplets.
//
// The function accepts FileInfo (rather than FileMode) to avoid referencing
// the io/fs.FileMode type directly, which is not in the import allowlist.
func formatMode(info iofs.FileInfo) string {
	var buf [10]byte
	mode := info.Mode()

	// Char 0: file type.
	// NOTE: Block device ('b') and character device ('c') types are
	// intentionally omitted because the syscall package (needed for
	// ModeDevice/ModeCharDevice) is banned by the import allowlist.
	switch {
	case mode&iofs.ModeDir != 0:
		buf[0] = 'd'
	case mode&iofs.ModeSymlink != 0:
		buf[0] = 'l'
	case mode&iofs.ModeNamedPipe != 0:
		buf[0] = 'p'
	case mode&iofs.ModeSocket != 0:
		buf[0] = 's'
	default:
		buf[0] = '-'
	}

	// Permission bits: extract the low 9 bits (rwxrwxrwx).
	const rwx = "rwx"
	perm := uint32(mode) & 0777
	for i := 0; i < 9; i++ {
		if perm&(1<<uint(8-i)) != 0 {
			buf[1+i] = rwx[i%3]
		} else {
			buf[1+i] = '-'
		}
	}

	// Setuid: affects owner execute (buf[3]).
	if mode&iofs.ModeSetuid != 0 {
		if buf[3] == 'x' {
			buf[3] = 's'
		} else {
			buf[3] = 'S'
		}
	}

	// Setgid: affects group execute (buf[6]).
	if mode&iofs.ModeSetgid != 0 {
		if buf[6] == 'x' {
			buf[6] = 's'
		} else {
			buf[6] = 'S'
		}
	}

	// Sticky: affects other execute (buf[9]).
	if mode&iofs.ModeSticky != 0 {
		if buf[9] == 'x' {
			buf[9] = 't'
		} else {
			buf[9] = 'T'
		}
	}

	return string(buf[:])
}

func indicator(info iofs.FileInfo, opts *options) string {
	mode := info.Mode()
	if opts.classify {
		if mode.IsDir() {
			return "/"
		}
		if mode&iofs.ModeSymlink != 0 {
			return "@"
		}
		if mode&iofs.ModeNamedPipe != 0 {
			return "|"
		}
		if mode&iofs.ModeSocket != 0 {
			return "="
		}
		if mode&0111 != 0 { // executable
			return "*"
		}
		return ""
	}
	if opts.appendSlash && mode.IsDir() {
		return "/"
	}
	return ""
}

func sortEntries[T any](entries []T, opts *options, getInfo func(T) iofs.FileInfo, getName func(T) string) {
	if opts.sortSize {
		slices.SortFunc(entries, func(a, b T) int {
			sa, sb := getInfo(a).Size(), getInfo(b).Size()
			if sa != sb {
				// Largest first.
				if sa > sb {
					return -1
				}
				return 1
			}
			// Break ties alphabetically by name.
			na, nb := getName(a), getName(b)
			if na < nb {
				return -1
			}
			if na > nb {
				return 1
			}
			return 0
		})
	} else if opts.sortTime {
		slices.SortFunc(entries, func(a, b T) int {
			ta, tb := getInfo(a).ModTime(), getInfo(b).ModTime()
			if !ta.Equal(tb) {
				// Newest first.
				if ta.After(tb) {
					return -1
				}
				return 1
			}
			// Break ties alphabetically by name.
			na, nb := getName(a), getName(b)
			if na < nb {
				return -1
			}
			if na > nb {
				return 1
			}
			return 0
		})
	} else {
		// Default: sort alphabetically by name.
		slices.SortFunc(entries, func(a, b T) int {
			na, nb := getName(a), getName(b)
			if na < nb {
				return -1
			}
			if na > nb {
				return 1
			}
			return 0
		})
	}

	if opts.reverse {
		slices.Reverse(entries)
	}
}

func joinPath(dir, name string) string {
	if len(dir) == 0 {
		return name
	}
	last := dir[len(dir)-1]
	if last == '/' || (runtime.GOOS == "windows" && last == '\\') {
		return dir + name
	}
	return dir + "/" + name
}

func humanSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d", size)
	}
	units := []string{"K", "M", "G", "T", "P"}
	val := float64(size)
	for _, u := range units {
		val /= 1024
		if val < 1024 {
			if val < 10 {
				return fmt.Sprintf("%.1f%s", val, u)
			}
			return fmt.Sprintf("%.0f%s", val, u)
		}
	}
	return fmt.Sprintf("%.0fP", val)
}

func formatTime(t time.Time, now time.Time) string {
	sixMonthsAgo := now.AddDate(0, -6, 0)
	if t.Before(sixMonthsAgo) || t.After(now) {
		// Old or future: show year instead of time.
		return t.Format("Jan _2  2006")
	}
	return t.Format("Jan _2 15:04")
}
