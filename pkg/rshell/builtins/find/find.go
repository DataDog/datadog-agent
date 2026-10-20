// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package find implements the find builtin command.
//
// find — search for files in a directory hierarchy
//
// Usage: find [-H] [-L] [-P] [PATH...] [EXPRESSION]
//
// Search the directory tree rooted at each PATH, evaluating the given
// EXPRESSION for each file found. If no PATH is given, the current
// directory (.) is used. If no EXPRESSION is given, -print is implied.
//
// Global options:
//
//	--help  Print usage information and exit.
//	-L      Follow symbolic links.
//
// Supported predicates:
//
//	-name PATTERN    — basename matches shell glob PATTERN
//	-iname PATTERN   — like -name but case-insensitive
//	-path PATTERN    — full path matches shell glob PATTERN
//	-ipath PATTERN   — like -path but case-insensitive
//	-type TYPE       — file type: b,c,d,f,l,p,s. Comma-separated for OR.
//	-size N[cwbkMG]  — file size. +N = greater, -N = less, N = exact.
//	-empty           — empty regular file or directory
//	-newer FILE      — modified more recently than FILE
//	-mtime N         — modified N days ago (+N = more, -N = less)
//	-mmin N          — modified N minutes ago (+N = more, -N = less)
//	-perm MODE       — permission bits match MODE (octal or symbolic)
//	-maxdepth N      — descend at most N levels
//	-mindepth N      — apply tests only at depth >= N
//	-print           — print path followed by newline
//	-print0          — print path followed by NUL
//	-prune           — skip directory subtree
//	-exec CMD {} ;   — run CMD with full path
//	-execdir CMD {} ; — run CMD in file's directory with ./basename
//	-quit            — exit immediately
//	-true            — always true
//	-false           — always false
//
// Operators:
//
//	( EXPR )         — grouping
//	! EXPR, -not EXPR — negation
//	EXPR -a EXPR, EXPR -and EXPR, EXPR EXPR — conjunction (implicit)
//	EXPR -o EXPR, EXPR -or EXPR — disjunction
//
// Blocked predicates (sandbox safety):
//
//	-delete, -ok, -okdir — deletion/interactive
//	-fls, -fprint, -fprint0, -fprintf — file writes
//	-regex, -iregex — ReDoS risk
//
// Exit codes:
//
//	0  All paths searched successfully.
//	1  At least one error occurred.
package find

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// isNotExist checks whether an error represents a "not found" condition.
// The sandbox's PortablePathError wraps errors with errors.New(), stripping
// the fs.ErrNotExist sentinel, so we check both errors.Is and the string.
func isNotExist(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	// PortablePathError rewrites the inner error as a plain string;
	// check for the canonical portable message.
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error() == "no such file or directory"
	}
	return false
}

// maxTraversalDepth limits directory recursion depth to prevent resource
// exhaustion. This is an intentional safety divergence from GNU find (which
// has no depth limit): the shell is designed for AI agent use where safety
// is the primary goal. When the user provides -maxdepth exceeding this
// limit, a warning is emitted and the value is clamped. Without -maxdepth,
// this cap applies silently as a defense-in-depth measure.
const maxTraversalDepth = 256

// Cmd is the find builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "find",
	Description: "search for files in a directory hierarchy",
	MakeFlags:   builtins.NoFlags(run),
}

func run(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
	// Parse global options (-L) and separate paths from expression.
	followLinks := false
	i := 0

	// Parse leading global options.
optLoop:
	for i < len(args) {
		switch args[i] {
		case "--help":
			printHelp(callCtx)
			return builtins.Result{}
		case "-L":
			followLinks = true
			i++
		case "-P":
			// -P overrides any earlier -L (last option wins).
			followLinks = false
			i++
		case "-H":
			callCtx.Errf("find: -H is not supported\n")
			return builtins.Result{Code: 1}
		case "--":
			i++ // consume --; stop option parsing
			break optLoop
		default:
			break optLoop
		}
	}

	// Separate paths from expression. Paths are args before the first
	// expression token (anything starting with - or ! or ( or )).
	var paths []string
	for i < len(args) {
		arg := args[i]
		if isExpressionStart(arg) {
			break
		}
		paths = append(paths, filepath.ToSlash(arg))
		i++
	}

	if len(paths) == 0 {
		paths = []string{"."}
	}

	exprArgs := args[i:]

	// Parse expression (includes -maxdepth/-mindepth as parser-recognized
	// options). The recursive-descent parser naturally handles token ownership,
	// so depth options can appear in any position without stealing arguments
	// from other predicates.
	pr, err := parseExpression(exprArgs)
	if err != nil {
		if errors.Is(err, errHelpRequested) {
			printHelp(callCtx)
			return builtins.Result{}
		}
		callCtx.Errf("%s\n", err.Error())
		return builtins.Result{Code: 1}
	}
	expression := pr.expr

	maxDepth := pr.maxDepth
	if maxDepth < 0 {
		maxDepth = maxTraversalDepth
	}
	if maxDepth > maxTraversalDepth {
		callCtx.Errf("find: warning: -maxdepth %d exceeds safety limit %d; clamped to %d\n", maxDepth, maxTraversalDepth, maxTraversalDepth)
		maxDepth = maxTraversalDepth
	}
	minDepth := pr.minDepth
	if minDepth < 0 {
		minDepth = 0
	}

	// Post-parse validation: check -exec/-execdir commands are allowed.
	// Commands containing {} are skipped here — the substituted name is
	// validated at eval-time when the replacement is known.
	for _, cmd := range collectExecCmds(expression) {
		if strings.Contains(cmd, "{}") {
			continue
		}
		if callCtx.CommandAllowed != nil && !callCtx.CommandAllowed(cmd) {
			callCtx.Errf("find: '%s': command not allowed\n", builtins.SafeOperand(cmd))
			return builtins.Result{Code: 1}
		}
	}

	// If no explicit action, add implicit -print.
	implicitPrint := expression == nil || !hasAction(expression)

	// Eagerly validate -newer reference paths before walking.
	// GNU find reports missing reference files even if short-circuiting
	// or -mindepth prevents the predicate from being evaluated.
	// With -L, stat the reference (following symlinks) to get the target
	// mtime; fall back to lstat for dangling symlinks.
	failed := false
	eagerNewerErrors := map[string]bool{}
	seen := map[string]bool{}
	for _, ref := range collectNewerRefs(expression) {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		if ref == "" {
			callCtx.Errf("find: '': No such file or directory\n")
			eagerNewerErrors[ref] = true
			failed = true
			continue
		}
		statRef := callCtx.LstatFile
		if followLinks {
			statRef = callCtx.StatFile
		}
		if _, err := statRef(ctx, ref); err != nil {
			// With -L, stat fails on dangling symlinks — fall back to
			// lstat so the symlink's own mtime can be used. Only fall
			// back for "not found" errors; permission/sandbox errors
			// must be reported.
			if followLinks && isNotExist(err) {
				if _, lerr := callCtx.LstatFile(ctx, ref); lerr == nil {
					continue
				}
			}
			callCtx.Errf("find: '%s': %s\n", builtins.SafeOperand(ref), callCtx.PortableErr(err))
			eagerNewerErrors[ref] = true
			failed = true
		}
	}

	now := callCtx.Now

	// Resolve working directory for -execdir path computation.
	var workDir string
	if callCtx.WorkDir != nil {
		workDir = callCtx.WorkDir()
	}

	// GNU find treats a missing -newer reference as a fatal argument error
	// and produces no result set, so skip the walk entirely.
	if !failed {
		for _, startPath := range paths {
			if ctx.Err() != nil {
				failed = true
				break
			}
			// Reject empty path operands — GNU find treats "" as a
			// non-existent path but continues walking remaining paths.
			if startPath == "" {
				callCtx.Errf("find: '': No such file or directory\n")
				failed = true
				continue
			}
			wr := walkPath(ctx, callCtx, startPath, walkOptions{
				expression:       expression,
				implicitPrint:    implicitPrint,
				followLinks:      followLinks,
				maxDepth:         maxDepth,
				minDepth:         minDepth,
				now:              now,
				eagerNewerErrors: eagerNewerErrors,
				workDir:          workDir,
			})
			if wr.failed {
				failed = true
			}
			if wr.quit {
				break
			}
		}
	}

	if failed {
		return builtins.Result{Code: 1}
	}
	return builtins.Result{}
}

// isExpressionStart returns true if the argument starts a find expression.
// GNU find treats any dash-prefixed token with length > 1 as an expression
// token (not a path), so `-1` is an unknown predicate, not a path argument.
func isExpressionStart(arg string) bool {
	if arg == "!" || arg == "(" {
		return true
	}
	return strings.HasPrefix(arg, "-") && len(arg) > 1
}

// printHelp outputs the find usage information.
func printHelp(callCtx *builtins.CallContext) {
	callCtx.Out("Usage: find [-L] [-P] [path...] [expression]\n\n")
	callCtx.Out("Search directory trees, evaluating an expression for each file found.\n")
	callCtx.Out("Default path is the current directory; default expression is -print.\n\n")
	callCtx.Out("Options:\n")
	callCtx.Out("  --help                     Print this help and exit.\n")
	callCtx.Out("  -L                         Follow symbolic links.\n")
	callCtx.Out("  -P                         Never follow symbolic links (default).\n\n")
	callCtx.Out("Tests:\n")
	callCtx.Out("  -name PATTERN              Base name matches shell glob PATTERN.\n")
	callCtx.Out("  -iname PATTERN             Like -name but case-insensitive.\n")
	callCtx.Out("  -path PATTERN              Full path matches shell glob PATTERN.\n")
	callCtx.Out("  -ipath PATTERN             Like -path but case-insensitive.\n")
	callCtx.Out("  -type TYPE                 File type: b,c,d,f,l,p,s. Comma-separated for OR.\n")
	callCtx.Out("  -size N[cwbkMG]            File size (+N=greater, -N=less, N=exact).\n")
	callCtx.Out("  -empty                     Empty regular file or directory.\n")
	callCtx.Out("  -newer FILE                Modified more recently than FILE.\n")
	callCtx.Out("  -mtime N                   Modified N days ago (+N=more, -N=less).\n")
	callCtx.Out("  -mmin N                    Modified N minutes ago (+N=more, -N=less).\n")
	callCtx.Out("  -perm MODE                 Permission bits match MODE (octal or symbolic).\n")
	callCtx.Out("  -maxdepth N                Descend at most N levels.\n")
	callCtx.Out("  -mindepth N                Apply tests only at depth >= N.\n")
	callCtx.Out("  -true                      Always true.\n")
	callCtx.Out("  -false                     Always false.\n\n")
	callCtx.Out("Actions:\n")
	callCtx.Out("  -print                     Print path followed by newline.\n")
	callCtx.Out("  -print0                    Print path followed by NUL.\n")
	callCtx.Out("  -exec CMD [ARG]... ;       Run CMD with full path.\n")
	callCtx.Out("  -execdir CMD [ARG]... ;    Run CMD in file's directory (./basename).\n")
	callCtx.Out("  -prune                     Skip directory subtree.\n")
	callCtx.Out("  -quit                      Exit immediately.\n\n")
	callCtx.Out("Operators:\n")
	callCtx.Out("  ( EXPR )                   Grouping.\n")
	callCtx.Out("  ! EXPR / -not EXPR         Negation.\n")
	callCtx.Out("  EXPR -a EXPR / EXPR -and EXPR  Conjunction (implicit).\n")
	callCtx.Out("  EXPR -o EXPR / EXPR -or EXPR   Disjunction.\n\n")
	callCtx.Out("Blocked predicates [sandbox]:\n")
	callCtx.Out("  -delete, -ok, -okdir                           Deletion/interactive.\n")
	callCtx.Out("  -fls, -fprint, -fprint0, -fprintf              File writes.\n")
	callCtx.Out("  -regex, -iregex                                ReDoS risk.\n")
}

// walkOptions holds configuration for a single walkPath invocation.
type walkOptions struct {
	expression       *expr
	implicitPrint    bool
	followLinks      bool
	maxDepth         int
	minDepth         int
	now              time.Time
	eagerNewerErrors map[string]bool
	workDir          string // absolute working directory for -exec/-execdir path resolution
}

// walkResult holds the outcome of a walk operation.
type walkResult struct {
	failed bool // at least one error occurred
	quit   bool // -quit was triggered
}

// walkPath walks the directory tree rooted at startPath, evaluating the
// expression for each entry.
func walkPath(
	ctx context.Context,
	callCtx *builtins.CallContext,
	startPath string,
	opts walkOptions,
) walkResult {
	now := opts.now
	failed := false
	quit := false
	newerCache := map[string]time.Time{}
	newerErrors := map[string]bool{}
	for k, v := range opts.eagerNewerErrors {
		newerErrors[k] = v
	}

	// Stat the starting path.
	var startInfo iofs.FileInfo
	var err error
	if opts.followLinks {
		startInfo, err = callCtx.StatFile(ctx, startPath)
		if err != nil && isNotExist(err) {
			startInfo, err = callCtx.LstatFile(ctx, startPath)
		}
	} else {
		startInfo, err = callCtx.LstatFile(ctx, startPath)
	}
	if err != nil {
		callCtx.Errf("find: '%s': %s\n", builtins.SafeOperand(startPath), callCtx.PortableErr(err))
		return walkResult{failed: true}
	}

	// Cycle detection for -L mode: track ancestor directory identities
	// (dev+inode on Unix, volume serial+file index on Windows) along the
	// path from root to the current node.

	// dirIterator streams directory entries one at a time via ReadDir(1),
	// keeping memory usage proportional to tree depth, not directory width.
	type dirIterator struct {
		dir           iofs.ReadDirFile
		parentPath    string
		depth         int
		ancestorIDs   map[builtins.FileID]string
		ancestorPaths map[string]bool
		done          bool
	}

	// checkLoop detects symlink loops for -L mode.
	checkLoop := func(path string, info iofs.FileInfo, ancestorIDs map[builtins.FileID]string, ancestorPaths map[string]bool) (bool, map[builtins.FileID]string, map[string]bool) {
		var childAncestorIDs map[builtins.FileID]string
		var childAncestorPaths map[string]bool
		if info.IsDir() && opts.followLinks {
			idOK := false
			if callCtx.FileIdentity != nil {
				if id, ok := callCtx.FileIdentity(path, info); ok {
					idOK = true
					if firstPath, seen := ancestorIDs[id]; seen {
						callCtx.Errf("find: File system loop detected; '%s' is part of the same file system loop as '%s'.\n",
							builtins.SafeOperand(path), builtins.SafeOperand(firstPath))
						failed = true
						return true, nil, nil
					}
					childAncestorIDs = make(map[builtins.FileID]string, len(ancestorIDs)+1)
					for k, v := range ancestorIDs {
						childAncestorIDs[k] = v
					}
					childAncestorIDs[id] = path
				}
			}
			if !idOK {
				if ancestorPaths[path] {
					callCtx.Errf("find: File system loop detected; '%s' has already been visited.\n", builtins.SafeOperand(path))
					failed = true
					return true, nil, nil
				}
				childAncestorPaths = make(map[string]bool, len(ancestorPaths)+1)
				for k := range ancestorPaths {
					childAncestorPaths[k] = true
				}
				childAncestorPaths[path] = true
			}
		}
		return false, childAncestorIDs, childAncestorPaths
	}

	// processEntry evaluates the expression for a single file entry.
	// Returns (prune, quit).
	processEntry := func(path string, info iofs.FileInfo, depth int) (bool, bool) {
		// Compute parent directory for -execdir.
		// Use joinPath (not filepath.Join) to preserve '.' and '..' components.
		// filepath.Join cleans lexically, so Join(cwd, ".") = cwd and Dir(cwd)
		// would point one level above the search root.
		absPath := path
		if !filepath.IsAbs(absPath) {
			absPath = joinPath(opts.workDir, absPath)
		}
		execDirParent := execDirParentDir(absPath)

		ec := &evalContext{
			callCtx:       callCtx,
			ctx:           ctx,
			now:           now,
			relPath:       path,
			info:          info,
			depth:         depth,
			printPath:     path,
			newerCache:    newerCache,
			newerErrors:   newerErrors,
			followLinks:   opts.followLinks,
			execDirParent: execDirParent,
			execWorkDir:   opts.workDir,
		}

		prune := false
		if depth >= opts.minDepth {
			result := evaluate(ec, opts.expression)
			prune = result.prune
			if len(newerErrors) > 0 || ec.failed {
				failed = true
			}
			if result.quit {
				return prune, true
			}
			if result.matched && opts.implicitPrint {
				callCtx.Outf("%s\n", path)
			}
		}

		return prune, false
	}

	// Process the starting path.
	isLoop, childAncIDs, childAncPaths := checkLoop(startPath, startInfo, nil, nil)

	startPrune := false
	if !isLoop {
		var q bool
		startPrune, q = processEntry(startPath, startInfo, 0)
		if q {
			return walkResult{failed: failed, quit: true}
		}
	}

	// Set up the iterator stack. Each open directory keeps a file handle
	// that reads one entry at a time, so memory is O(depth) not O(width).
	var iterStack []*dirIterator

	if !isLoop && !startPrune && startInfo.IsDir() && 0 < opts.maxDepth {
		dir, openErr := callCtx.OpenDir(ctx, startPath)
		if openErr != nil {
			callCtx.Errf("find: '%s': %s\n", builtins.SafeOperand(startPath), callCtx.PortableErr(openErr))
			return walkResult{failed: true}
		}
		iterStack = append(iterStack, &dirIterator{
			dir:           dir,
			parentPath:    startPath,
			depth:         1,
			ancestorIDs:   childAncIDs,
			ancestorPaths: childAncPaths,
		})
	}

	for len(iterStack) > 0 {
		if ctx.Err() != nil || quit {
			failed = failed || ctx.Err() != nil
			break
		}

		top := iterStack[len(iterStack)-1]
		if top.done {
			top.dir.Close()
			iterStack = iterStack[:len(iterStack)-1]
			continue
		}

		dirEntries, readErr := top.dir.ReadDir(1)
		if readErr != nil {
			if readErr != io.EOF {
				callCtx.Errf("find: '%s': %s\n", builtins.SafeOperand(top.parentPath), callCtx.PortableErr(readErr))
				failed = true
			}
			top.done = true
			continue
		}
		if len(dirEntries) == 0 {
			top.done = true
			continue
		}

		child := dirEntries[0]
		childPath := joinPath(top.parentPath, child.Name())

		var childInfo iofs.FileInfo
		if opts.followLinks {
			childInfo, err = callCtx.StatFile(ctx, childPath)
			if err != nil && isNotExist(err) {
				childInfo, err = callCtx.LstatFile(ctx, childPath)
			}
			if err != nil {
				callCtx.Errf("find: '%s': %s\n", builtins.SafeOperand(childPath), callCtx.PortableErr(err))
				failed = true
				continue
			}
		} else {
			childInfo, err = callCtx.LstatFile(ctx, childPath)
			if err != nil {
				callCtx.Errf("find: '%s': %s\n", builtins.SafeOperand(childPath), callCtx.PortableErr(err))
				failed = true
				continue
			}
		}

		isLoop, cAncIDs, cAncPaths := checkLoop(childPath, childInfo, top.ancestorIDs, top.ancestorPaths)
		if isLoop {
			continue
		}

		prune, q := processEntry(childPath, childInfo, top.depth)
		if q {
			quit = true
			break
		}

		// Descend into child directories unless pruned or beyond maxdepth.
		if childInfo.IsDir() && !prune && top.depth < opts.maxDepth {
			dir, openErr := callCtx.OpenDir(ctx, childPath)
			if openErr != nil {
				callCtx.Errf("find: '%s': %s\n", builtins.SafeOperand(childPath), callCtx.PortableErr(openErr))
				failed = true
				continue
			}
			iterStack = append(iterStack, &dirIterator{
				dir:           dir,
				parentPath:    childPath,
				depth:         top.depth + 1,
				ancestorIDs:   cAncIDs,
				ancestorPaths: cAncPaths,
			})
		}
	}

	// Close any remaining open directory handles (e.g. on context cancellation).
	for _, it := range iterStack {
		it.dir.Close()
	}

	return walkResult{failed: failed, quit: quit}
}

// collectExecCmds walks the expression tree and returns all -exec/-execdir command names.
func collectExecCmds(e *expr) []string {
	var cmds []string
	collectExecCmdsInto(e, &cmds)
	return cmds
}

func collectExecCmdsInto(e *expr, cmds *[]string) {
	if e == nil {
		return
	}
	if e.kind == exprExecDir || e.kind == exprExec {
		*cmds = append(*cmds, e.execCmd)
	}
	collectExecCmdsInto(e.left, cmds)
	collectExecCmdsInto(e.right, cmds)
	collectExecCmdsInto(e.operand, cmds)
}

// collectNewerRefs walks the expression tree and returns all -newer reference paths.
func collectNewerRefs(e *expr) []string {
	if e == nil {
		return nil
	}
	if e.kind == exprNewer {
		return []string{e.strVal}
	}
	var refs []string
	refs = append(refs, collectNewerRefs(e.left)...)
	refs = append(refs, collectNewerRefs(e.right)...)
	refs = append(refs, collectNewerRefs(e.operand)...)
	return refs
}

// joinPath joins a directory and a name with a forward slash.
// The shell normalises all paths to forward slashes on all platforms,
// so hardcoding '/' is correct even on Windows.
func joinPath(dir, name string) string {
	if len(dir) == 0 {
		return name
	}
	if dir[len(dir)-1] == '/' {
		return dir + name
	}
	return dir + "/" + name
}

// execDirParentDir returns the parent directory of absPath for -execdir.
// Trailing slashes are trimmed (except Unix root "/" and Windows drive
// roots like "C:/") so that filepath.Dir returns the parent directory
// rather than the directory itself.
func execDirParentDir(absPath string) string {
	for len(absPath) > 1 && absPath[len(absPath)-1] == '/' {
		if len(absPath) == 3 && absPath[1] == ':' {
			break // preserve Windows drive root (e.g. C:/)
		}
		absPath = absPath[:len(absPath)-1]
	}
	return filepath.Dir(absPath)
}
