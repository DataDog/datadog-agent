// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package pwd implements the pwd builtin command.
//
// pwd — print the absolute pathname of the current working directory
//
// Usage: pwd [-LP] [--help]
//
// Print the absolute pathname of the shell's current working directory
// to standard output. Stdin is not used. Extra positional arguments are
// ignored to match GNU coreutils and bash behavior.
//
// Flags:
//
//	-L, --logical   Use the shell's tracked logical path (default).
//	                The path may contain symbolic links, exactly as the
//	                user reached the directory. This matches the POSIX
//	                default and the bash builtin default.
//	-P, --physical  Resolve all symbolic links so the printed path contains
//	                no symlinks and no "." or ".." components.
//	-h, --help      Print usage to stdout and exit 0.
//
// If both -L and -P are given on the same invocation, the last one wins
// (matches POSIX and bash, implemented via pflag's declaration-order
// Visit traversal).
//
// Symlink resolution for -P is sandbox-safe: it walks the absolute path
// component-by-component using callCtx.LstatFile and callCtx.ReadlinkFile,
// never calling os.Lstat / os.Readlink directly. The total number of
// symlink expansions is capped at maxSymlinkHops (40) to defeat cycles,
// matching the Linux ELOOP limit.
//
// Sandbox best-effort for -P: if a path component lies outside the
// AllowedPaths sandbox (a common case when the working directory is
// itself the sandbox root), the resolver cannot walk that component and
// gives up gracefully — falling back to the logical path. This means
// "pwd -P" never produces a hard error from the sandbox; it returns the
// best canonical path it can compute.
//
// Exit codes:
//
//	0  Success — a working directory was written (logical or physical).
//	1  Error — invalid flag, or the runner exposes no working directory.
package pwd

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the pwd builtin command descriptor. Help is intentionally not
// set: pwd registers flags, so the `help` builtin invokes `pwd --help`
// to produce its description, keeping `help pwd` and `pwd --help`
// identical.
var Cmd = builtins.Command{
	Name:        "pwd",
	Description: "print working directory",
	MakeFlags:   makeFlags,
}

// maxSymlinkHops caps the number of symlink expansions during -P
// resolution. Matches the Linux ELOOP limit (40) so that pathological
// or cyclic chains terminate with a clear error.
const maxSymlinkHops = 40

// errSymlinkLoop is returned when -P resolution exceeds maxSymlinkHops.
var errSymlinkLoop = errors.New("too many levels of symbolic links")

// boolSeqSentinel is the NoOptDefVal we register for -L/-P. pflag passes
// this exact string to Set when the user types the bare flag form
// (`pwd -P`). Any other value — including the literal "true" supplied
// via `--physical=true` — fails the equality check in Set and is
// rejected as "option doesn't allow an argument", matching GNU.
const boolSeqSentinel = "\x00rshell:pwd:bare\x00"

func makeFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	helpFlag := fs.BoolP("help", "h", false, "print usage and exit")

	// -L and -P share a sequence counter so that after parsing we can
	// compare their pos fields to determine which appeared last on the
	// command line. pflag calls Set() in parse order, so the last flag
	// Set gets the highest pos value. We do this rather than relying on
	// pflag.FlagSet.Visit because Visit walks in lexicographical (or
	// declaration) order — never command-line order — which would make
	// `-P -L` always pick the wrong mode.
	//
	// boolSeqFlag.Set also rejects explicit values (e.g. --logical=foo
	// or --physical=false). NoOptDefVal="true" lets pflag accept the
	// bare flag form; any other value yields an error, matching GNU
	// `/bin/pwd --physical=false`.
	var modeSeq int
	logicalFlag := newBoolSeqFlag(&modeSeq)
	physicalFlag := newBoolSeqFlag(&modeSeq)
	fs.VarPF(logicalFlag, "logical", "L", "use the shell's working directory, even if it contains symlinks (default)").NoOptDefVal = boolSeqSentinel
	fs.VarPF(physicalFlag, "physical", "P", "resolve all symlinks before printing the path").NoOptDefVal = boolSeqSentinel

	return func(ctx context.Context, callCtx *builtins.CallContext, _ []string) builtins.Result {
		if *helpFlag {
			printHelp(callCtx)
			return builtins.Result{}
		}

		if callCtx.WorkDir == nil {
			callCtx.Errf("pwd: no working directory available\n")
			return builtins.Result{Code: 1}
		}

		cwd := callCtx.WorkDir()
		if cwd == "" {
			callCtx.Errf("pwd: working directory is empty\n")
			return builtins.Result{Code: 1}
		}

		if physicalFlag.pos > logicalFlag.pos {
			// Best-effort: if symlink resolution fails (typically because
			// the cwd is the sandbox root and we cannot walk above it),
			// fall back to the logical path silently. Cycles still error
			// because they indicate corrupt input, not a sandbox limit.
			//
			// Context cancellation is *not* a best-effort case: if the
			// run is being interrupted, we must not report success with
			// a stale logical path. RULES.md requires graceful handling
			// of cancellation; we exit 1 without writing.
			resolved, err := resolveSymlinks(ctx, callCtx, cwd)
			switch {
			case err == nil:
				cwd = resolved
			case errors.Is(err, errSymlinkLoop):
				callCtx.Errf("pwd: %s\n", err)
				return builtins.Result{Code: 1}
			case ctx.Err() != nil:
				return builtins.Result{Code: 1}
			}
			// If the AllowedPaths root containing cwd is itself a
			// symlink, os.Root has already followed it at sandbox
			// init, but the per-component walk in resolveSymlinks
			// cannot see that resolution (LstatFile sees the opened
			// target dir, not the original link). Translate the root
			// prefix here so `pwd -P` reflects the resolution that
			// the sandbox is already enforcing under the hood.
			if callCtx.CanonicalizeRootPrefix != nil {
				cwd = callCtx.CanonicalizeRootPrefix(cwd)
			}
		}

		callCtx.Outf("%s\n", cwd)
		return builtins.Result{}
	}
}

// boolSeqFlag is a pflag.Value implementation for -L/-P. Two boolSeqFlag
// values share a *seq counter; each call to Set increments the counter
// and records the new value in pos. After pflag.Parse, comparing pos
// fields reveals which flag appeared last on the command line.
//
// Set also rejects explicit values: pwd's mode flags are bare boolean
// switches and `pwd --physical=false` should fail like GNU coreutils
// rather than silently flip the mode.
type boolSeqFlag struct {
	seq *int
	pos int
}

func newBoolSeqFlag(seq *int) *boolSeqFlag {
	return &boolSeqFlag{seq: seq}
}

func (f *boolSeqFlag) String() string { return "false" }

func (f *boolSeqFlag) Set(s string) error {
	// pflag passes NoOptDefVal (boolSeqSentinel) when the user types the
	// bare flag form, and the user-supplied value for `--flag=value`.
	// Reject anything that isn't the sentinel — including the literal
	// "true", which GNU `/bin/pwd --physical=true` also rejects.
	if s != boolSeqSentinel {
		return errors.New("option doesn't allow an argument")
	}
	*f.seq++
	f.pos = *f.seq
	return nil
}

func (f *boolSeqFlag) Type() string { return "bool" }

func printHelp(callCtx *builtins.CallContext) {
	callCtx.Out("Usage: pwd [-LP] [--help]\n")
	callCtx.Out("Print the absolute pathname of the current working directory.\n\n")
	callCtx.Out("  -L, --logical   use the shell's tracked working directory, even if it\n")
	callCtx.Out("                  contains symlinks (default)\n")
	callCtx.Out("  -P, --physical  resolve all symlinks before printing the path\n")
	callCtx.Out("  -h, --help      display this help and exit\n")
}

// resolveSymlinks returns the canonical (no-symlinks, no "." or "..")
// absolute form of path. It walks path component-by-component using the
// sandbox-safe LstatFile and ReadlinkFile from callCtx, so it never
// touches the filesystem directly. The number of symlink expansions is
// capped at maxSymlinkHops to defeat cycles.
//
// Algorithm: maintain `out` (the already-resolved prefix) and `rest`
// (the remaining unresolved suffix, always relative). Pop one component
// off rest at a time. If the component is a symlink, prepend the link
// target back onto rest (and reset out to the absolute root if the
// target is absolute). Continue until rest is empty.
func resolveSymlinks(ctx context.Context, callCtx *builtins.CallContext, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("not an absolute path: %s", path)
	}
	if callCtx.LstatFile == nil || callCtx.ReadlinkFile == nil {
		return "", errors.New("sandbox does not support symlink resolution")
	}

	cleaned := filepath.Clean(path)
	out := rootPrefix(cleaned)
	rest := strings.TrimPrefix(cleaned, out)

	hops := 0
	for rest != "" {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		// Consume any leading separator(s) before extracting the next
		// component. filepath.Clean collapses runs of separators, but
		// expanded symlink targets (which we splice back into rest)
		// can reintroduce them.
		for len(rest) > 0 && rest[0] == filepath.Separator {
			rest = rest[1:]
		}
		if rest == "" {
			break
		}

		var comp string
		if i := strings.IndexByte(rest, filepath.Separator); i >= 0 {
			comp = rest[:i]
			rest = rest[i:]
		} else {
			comp = rest
			rest = ""
		}

		switch comp {
		case ".":
			continue
		case "..":
			out = parentDir(out)
			continue
		}

		candidate := joinPath(out, comp)
		info, err := callCtx.LstatFile(ctx, candidate)
		if err != nil {
			// Cannot stat through the sandbox — typically because the
			// path is above the AllowedPaths root. Treat the component
			// as opaque (not a symlink) and continue. This is what makes
			// -P resolution work when the cwd is somewhere under the
			// sandbox root: components above the root pass through, and
			// only the symlinks we can actually inspect get resolved.
			out = candidate
			continue
		}

		if info.Mode()&iofs.ModeSymlink == 0 {
			out = candidate
			continue
		}

		hops++
		if hops > maxSymlinkHops {
			return "", errSymlinkLoop
		}

		target, err := callCtx.ReadlinkFile(ctx, candidate)
		if err != nil {
			// Lstat said it's a symlink but readlink failed. Treat the
			// component as opaque rather than aborting resolution.
			out = candidate
			continue
		}

		if filepath.IsAbs(target) {
			cleanedTarget := filepath.Clean(target)
			// Container-style sandboxes mount the host filesystem at a
			// prefix (e.g. /mnt/host). Symlink targets stored on disk
			// often refer to host-absolute paths without that prefix
			// (e.g. /var/log/pods/...), so apply the prefix when set
			// — otherwise the resolved path would not be reachable
			// through the sandbox and `pwd -P` output would be unusable
			// for further filesystem operations.
			if callCtx.HostPrefix != nil {
				if hp := callCtx.HostPrefix(); hp != "" && !strings.HasPrefix(cleanedTarget, hp+string(filepath.Separator)) && cleanedTarget != hp {
					cleanedTarget = filepath.Join(hp, cleanedTarget)
				}
			}
			out = rootPrefix(cleanedTarget)
			rest = strings.TrimPrefix(cleanedTarget, out) + rest
		} else {
			// filepath.Clean normalizes the relative target — collapsing
			// "." segments and converting forward slashes to the host
			// separator on Windows. Without it, a target like "./real"
			// keeps its forward slash on Windows and the walking loop
			// (which splits on filepath.Separator) treats "./real" as a
			// single opaque component.
			rest = string(filepath.Separator) + filepath.Clean(target) + rest
		}
	}

	if out == "" {
		out = string(filepath.Separator)
	}
	return out, nil
}

// rootPrefix returns the leading absolute-path root for path. On Unix
// this is always "/". On Windows it preserves the drive letter or UNC
// volume so subsequent component walking does not mistake "C:" for a
// path component.
func rootPrefix(path string) string {
	vol := filepath.VolumeName(path)
	if vol != "" {
		if len(path) > len(vol) && path[len(vol)] == filepath.Separator {
			return vol + string(filepath.Separator)
		}
		return vol
	}
	return string(filepath.Separator)
}

// parentDir returns the directory containing dir, preserving the volume
// root. If dir is already at the root, parentDir returns dir unchanged.
func parentDir(dir string) string {
	root := rootPrefix(dir)
	if dir == root {
		return dir
	}
	parent := filepath.Dir(dir)
	if parent == "." {
		return root
	}
	return parent
}

// joinPath joins dir and a single non-empty component without using
// filepath.Join (which collapses ".." and may erase volume roots).
func joinPath(dir, comp string) string {
	if dir == "" {
		return comp
	}
	if dir[len(dir)-1] == filepath.Separator {
		return dir + comp
	}
	return dir + string(filepath.Separator) + comp
}
