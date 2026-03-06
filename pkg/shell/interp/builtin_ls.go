// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"cmp"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// lsEntry holds metadata about a single file/directory to be listed.
type lsEntry struct {
	name       string      // display name (basename or relative path)
	path       string      // absolute path for stat calls
	info       fs.FileInfo // stat result
	linkTarget string      // symlink target (populated in -l mode)
}

// builtinLs implements the POSIX ls command.
func (r *Runner) builtinLs(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	// Parse flags.
	var (
		showAll     bool // -a: show all including . and ..
		almostAll   bool // -A: show all except . and ..
		longFormat  bool // -l: long listing
		listDirSelf bool // -d: list directory entries themselves
		classify    bool // -F: append type indicator
		reverseSort bool // -r: reverse sort order
		sortByTime  bool // -t: sort by modification time
		sortBySize  bool // -S: sort by size
		recursive   bool // -R: recursive listing
		onePerLine  bool // -1: one entry per line (default for non-terminal)
		showInode   bool // -i: print inode number
		slashDirs   bool // -p: append / to directories
		numericIDs  bool // -n: numeric uid/gid (implies -l)
		derefAll    bool // -L: dereference all symlinks
		derefArgs   bool // -H: dereference symlinks on command line
	)

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-a":
			showAll = true
		case "-A":
			almostAll = true
		case "-l":
			longFormat = true
		case "-d":
			listDirSelf = true
		case "-F":
			classify = true
		case "-r":
			reverseSort = true
		case "-t":
			sortByTime = true
		case "-S":
			sortBySize = true
		case "-R":
			recursive = true
		case "-1":
			onePerLine = true
		case "-i":
			showInode = true
		case "-p":
			slashDirs = true
		case "-n":
			numericIDs = true
			longFormat = true
		case "-L":
			derefAll = true
		case "-H":
			derefArgs = true
		default:
			r.errf("ls: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}
	_ = onePerLine // output is always one-per-line in this implementation

	paths := fp.args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	// statFn selects stat vs lstat depending on -L flag.
	statFn := r.lstat
	if derefAll {
		statFn = r.stat
	}

	// Separate files from directories (POSIX: files listed first).
	var fileEntries []lsEntry
	var dirPaths []string

	for _, p := range paths {
		absP := r.absPath(p)

		// For command-line args, -H dereferences symlinks.
		sf := statFn
		if derefArgs && !derefAll {
			sf = r.stat
		}

		info, err := sf(ctx, absP)
		if err != nil {
			// Try lstat as fallback for broken symlinks.
			if derefAll || derefArgs {
				info, err = r.lstat(ctx, absP)
			}
			if err != nil {
				r.errf("ls: cannot access '%s': %v\n", p, err)
				exit.code = 1
				continue
			}
		}

		if info.IsDir() && !listDirSelf {
			dirPaths = append(dirPaths, p)
		} else {
			fileEntries = append(fileEntries, lsEntry{
				name: p,
				path: absP,
				info: info,
			})
		}
	}

	// Sort and print file entries.
	lsSortEntries(fileEntries, sortByTime, sortBySize, reverseSort)
	for _, e := range fileEntries {
		if longFormat {
			e.linkTarget = lsReadLink(e.path, e.info)
		}
		r.lsPrintEntry(e, longFormat, showInode, classify, slashDirs, numericIDs)
	}

	// Print directory entries.
	multipleArgs := len(fileEntries) > 0 || len(dirPaths) > 1
	for i, p := range dirPaths {
		if i > 0 || len(fileEntries) > 0 {
			r.out("\n")
		}
		if multipleArgs || recursive {
			r.outf("%s:\n", p)
		}
		r.lsDir(ctx, p, r.absPath(p), 0, &exit, lsDirOpts{
			showAll:     showAll,
			almostAll:   almostAll,
			longFormat:  longFormat,
			classify:    classify,
			reverseSort: reverseSort,
			sortByTime:  sortByTime,
			sortBySize:  sortBySize,
			recursive:   recursive,
			showInode:   showInode,
			slashDirs:   slashDirs,
			numericIDs:  numericIDs,
			derefAll:    derefAll,
			multiArgs:   multipleArgs,
		})
	}

	return exit
}

// lsDirOpts bundles the flags needed when listing a directory.
type lsDirOpts struct {
	showAll     bool
	almostAll   bool
	longFormat  bool
	classify    bool
	reverseSort bool
	sortByTime  bool
	sortBySize  bool
	recursive   bool
	showInode   bool
	slashDirs   bool
	numericIDs  bool
	derefAll    bool
	multiArgs   bool
}

// lsDir lists the contents of a single directory.
func (r *Runner) lsDir(ctx context.Context, displayPath, absPath string, depth int, exit *exitStatus, opts lsDirOpts) {
	const maxDepth = 256
	if depth > maxDepth {
		r.errf("ls: recursion depth exceeded for '%s'\n", displayPath)
		exit.code = 1
		return
	}

	dirEntries, err := r.readDirHandler(r.handlerCtx(ctx, handlerKindReadDir, todoPos), absPath)
	if err != nil {
		r.errf("ls: cannot open directory '%s': %v\n", displayPath, err)
		exit.code = 1
		return
	}

	statFn := r.lstat
	if opts.derefAll {
		statFn = r.stat
	}

	var entries []lsEntry

	// Manually add . and .. for -a.
	if opts.showAll {
		for _, dot := range []string{".", ".."} {
			var dotPath string
			if dot == "." {
				dotPath = absPath
			} else {
				dotPath = filepath.Dir(absPath)
			}
			info, err := r.lstat(ctx, dotPath)
			if err != nil {
				continue
			}
			entries = append(entries, lsEntry{name: dot, path: dotPath, info: info})
		}
	}

	for _, de := range dirEntries {
		name := de.Name()

		// Filter hidden files.
		if strings.HasPrefix(name, ".") && !opts.showAll && !opts.almostAll {
			continue
		}

		entryPath := filepath.Join(absPath, name)
		info, err := statFn(ctx, entryPath)
		if err != nil {
			// Broken symlink fallback.
			info, err = r.lstat(ctx, entryPath)
			if err != nil {
				r.errf("ls: cannot access '%s': %v\n", filepath.Join(displayPath, name), err)
				exit.code = 1
				continue
			}
		}
		entries = append(entries, lsEntry{name: name, path: entryPath, info: info})
	}

	lsSortEntries(entries, opts.sortByTime, opts.sortBySize, opts.reverseSort)

	// For long format, compute total blocks.
	if opts.longFormat {
		var totalBlocks int64
		for _, e := range entries {
			totalBlocks += lsFileBlocks(e.info)
		}
		r.outf("total %d\n", totalBlocks)
	}

	// Print entries.
	for _, e := range entries {
		if opts.longFormat {
			e.linkTarget = lsReadLink(e.path, e.info)
		}
		r.lsPrintEntry(e, opts.longFormat, opts.showInode, opts.classify, opts.slashDirs, opts.numericIDs)
	}

	// Recurse into subdirectories for -R.
	if opts.recursive {
		for _, e := range entries {
			if !e.info.IsDir() || e.name == "." || e.name == ".." {
				continue
			}
			subDisplay := filepath.Join(displayPath, e.name)
			r.outf("\n%s:\n", subDisplay)
			r.lsDir(ctx, subDisplay, e.path, depth+1, exit, opts)
		}
	}
}

// lsPrintEntry prints a single ls entry to stdout.
func (r *Runner) lsPrintEntry(e lsEntry, longFormat, showInode, classify, slashDirs, numericIDs bool) {
	var line strings.Builder

	if showInode {
		_, _, _, inode := lsFileOwnership(e.info, false)
		fmt.Fprintf(&line, "%d ", inode)
	}

	if longFormat {
		line.WriteString(lsFormatLong(e, numericIDs))
	} else {
		line.WriteString(e.name)
	}

	if classify {
		line.WriteString(lsClassifyIndicator(e.info.Mode()))
	} else if slashDirs && e.info.IsDir() {
		line.WriteString("/")
	}

	line.WriteString("\n")
	r.out(line.String())
}

// lsFormatLong builds one long-format line: mode nlink owner group size date name [-> target].
func lsFormatLong(e lsEntry, numericIDs bool) string {
	mode := e.info.Mode()
	owner, group, nlink, _ := lsFileOwnership(e.info, numericIDs)
	size := e.info.Size()
	mtime := e.info.ModTime()

	var b strings.Builder
	fmt.Fprintf(&b, "%s %3d %-8s %-8s %8d %s %s",
		mode.String(),
		nlink,
		owner,
		group,
		size,
		lsFormatTime(mtime),
		e.name,
	)

	if mode&fs.ModeSymlink != 0 && e.linkTarget != "" {
		fmt.Fprintf(&b, " -> %s", e.linkTarget)
	}

	return b.String()
}

// lsClassifyIndicator returns the -F suffix character for the given mode.
func lsClassifyIndicator(m os.FileMode) string {
	switch {
	case m.IsDir():
		return "/"
	case m&fs.ModeSymlink != 0:
		return "@"
	case m&fs.ModeNamedPipe != 0:
		return "|"
	case m&fs.ModeSocket != 0:
		return "="
	case m&0o111 != 0:
		return "*"
	default:
		return ""
	}
}

// lsFormatTime formats mtime in POSIX style:
// "Jan  2 15:04" for recent files (within 6 months), "Jan  2  2006" for older.
func lsFormatTime(t time.Time) string {
	now := time.Now()
	sixMonthsAgo := now.AddDate(0, -6, 0)
	if t.Before(sixMonthsAgo) || t.After(now) {
		return t.Format("Jan _2  2006")
	}
	return t.Format("Jan _2 15:04")
}

// lsSortEntries sorts entries by name, optionally by time or size, with optional reversal.
func lsSortEntries(entries []lsEntry, byTime, bySize, reverse bool) {
	slices.SortFunc(entries, func(a, b lsEntry) int {
		var c int
		switch {
		case byTime:
			c = b.info.ModTime().Compare(a.info.ModTime()) // newest first
		case bySize:
			c = cmp.Compare(b.info.Size(), a.info.Size()) // largest first
		default:
			c = cmp.Compare(a.name, b.name)
		}
		if reverse {
			c = -c
		}
		return c
	})
}

// lsReadLink reads the symlink target if the entry is a symlink.
func lsReadLink(path string, info fs.FileInfo) string {
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return ""
		}
		return target
	}
	return ""
}
