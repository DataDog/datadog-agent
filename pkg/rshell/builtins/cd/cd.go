// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package cd implements the cd builtin command.
//
// cd — change the shell's working directory
//
// Usage: cd [-LP] [-|DIRECTORY]
//
// Change the shell's current working directory. The new directory must
// resolve to a path inside the AllowedPaths sandbox; targets outside the
// sandbox are rejected with exit 1 and the working directory is left
// unchanged.
//
// Operands:
//
//	(none)       Change to the directory named by $HOME. If HOME is not
//	             set, an error is printed and exit 1 is returned.
//	-            Change to the directory named by $OLDPWD. If OLDPWD is
//	             not set (no successful cd has happened yet in this
//	             session), an error is printed and exit 1 is returned.
//	             On success the new absolute path is written to stdout
//	             (matching POSIX / bash).
//	DIRECTORY    Change to DIRECTORY. Relative paths are resolved against
//	             the shell's current working directory.
//
// Flags:
//
//	-L, --logical   Resolve `..` lexically; symbolic links in the result
//	                are preserved. This is the default and matches POSIX.
//	-P, --physical  Resolve symbolic links before processing `..`. The
//	                resolution is sandbox-safe — components the sandbox
//	                cannot inspect (paths above the AllowedPaths root,
//	                or the root path itself if it is a symlink) pass
//	                through unresolved.
//	-h, --help      Print usage to stdout and exit 0.
//
// If both -L and -P are given on the same invocation, the last one wins
// (matches POSIX and bash). Multiple positional operands are rejected
// with exit 1.
//
// Side effects on success:
//
//	$OLDPWD is set to the previous working directory.
//	$PWD    is set to the new working directory.
//	The shell's tracked working directory is updated; subsequent file
//	operations resolve relative paths against the new directory.
//
// Exit codes:
//
//	0  Success — the working directory was changed.
//	1  Error — invalid flag, missing target, target outside AllowedPaths,
//	   target is not a directory, or HOME/OLDPWD was required but unset.
//
// Bash extensions intentionally not implemented:
//
//	-e          Only meaningful with -P when getcwd fails; not reachable
//	            here because rshell maintains the working directory
//	            internally via runner.Dir.
//	-@          AIX-only extended-attribute view.
//	CDPATH      Implicit directory search via $CDPATH is a path-confusion
//	            risk in a restricted shell.
//	~user       User-home expansion is a parser concern, not cd's.
//	cdable_vars Bash extension that treats unknown directories as variable
//	            names; off by default in bash too.
//
// Sandbox-spanning invariants (`-P` divergence from bash):
//
//	The per-component walker for `-P` mode only resolves symlinks whose
//	parent component is inside `AllowedPaths`. Lstat against an
//	above-sandbox path returns `ErrPermission`, which the walker treats
//	as an opaque pass-through — so a sandbox-internal target reached
//	through an above-sandbox symlink (e.g. on macOS where `/tmp` is a
//	symlink to `/private/tmp`) is *not* resolved for that hop. The
//	final `changeDir` still rejects results outside the sandbox, so
//	this is a divergence from bash (which would resolve every hop),
//	not a security weakening. Operators running cd in a container-style
//	sandbox where the AllowedPaths root itself is a symlink should
//	expect cd -P to behave like cd -L for that root segment.
package cd

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the cd builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "cd",
	Description: "change the working directory",
	MakeFlags:   makeFlags,
}

// maxSymlinkHops bounds the number of symlink expansions done in -P
// mode. Mirrors the Linux ELOOP limit and matches pwd -P. Cyclic
// chains terminate with a clear error rather than spinning.
const maxSymlinkHops = 40

// errSymlinkLoop is returned when -P resolution exceeds maxSymlinkHops.
var errSymlinkLoop = errors.New("too many levels of symbolic links")

// errNotDirectory is returned by resolvePath when ".." processing
// requires a directory but the prefix is not one (e.g. `cd file/..`).
// The same sentinel exists in interp/cd_support.go for the final
// ChangeDir validation; defining a local copy keeps cd.go free of an
// interp dependency.
var errNotDirectory = errors.New("not a directory")

// errBoolSeqValue is returned by boolSeqFlag.Set when the user passes
// an explicit value to -L / -P (e.g. `cd --physical=true`). Defined at
// package scope so each invocation reuses the same error value rather
// than allocating per-call.
var errBoolSeqValue = errors.New("option doesn't allow an argument")

// boolSeqSentinel is the NoOptDefVal we register for -L/-P. pflag passes
// this exact string to Set when the user types the bare flag form
// (`cd -P`). Any other value — including the literal "true" supplied via
// `--physical=true` — fails the equality check in Set and is rejected
// as "option doesn't allow an argument", matching bash.
const boolSeqSentinel = "\x00rshell:cd:bare\x00"

func makeFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	helpFlag := fs.BoolP("help", "h", false, "print usage and exit")

	// Disable interspersed flag parsing: once the directory operand
	// is seen, any following token (including flag-shaped ones like
	// `-P`) must be treated as another positional. bash 5.2 rejects
	// `cd /tmp -P` with "too many arguments"; pflag's default would
	// silently consume the trailing -P as a flag. Setting this to
	// false makes the first positional terminate flag parsing.
	fs.SetInterspersed(false)

	// -L and -P share a sequence counter so that after parsing we can
	// compare their pos fields to determine which appeared last on the
	// command line. pflag calls Set() in parse order, so the last flag
	// Set gets the highest pos value. This mirrors pwd's pattern.
	var modeSeq int
	logicalFlag := newBoolSeqFlag(&modeSeq)
	physicalFlag := newBoolSeqFlag(&modeSeq)
	fs.VarPF(logicalFlag, "logical", "L", "treat .. lexically; preserve symbolic links in the result (default)").NoOptDefVal = boolSeqSentinel
	fs.VarPF(physicalFlag, "physical", "P", "resolve symbolic links before treating ..").NoOptDefVal = boolSeqSentinel

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *helpFlag {
			printHelp(callCtx)
			return builtins.Result{}
		}

		if callCtx.ChangeDir == nil || callCtx.WorkDir == nil {
			// ChangeDir is intentionally nil for RunCommand-spawned
			// children (find -exec, find -execdir, xargs) so a cd
			// inside a sub-command cannot leak into the parent shell.
			// Surface that to the user rather than leaving them
			// guessing why cd "is not supported".
			callCtx.Errf("cd: cannot change directory from inside find -exec/-execdir or xargs (child invocations are isolated)\n")
			return builtins.Result{Code: 1}
		}

		if len(args) > 1 {
			callCtx.Errf("cd: too many arguments\n")
			return builtins.Result{Code: 1}
		}

		// Resolve the operand into a target path string.
		// printResult is true when the new directory must be printed
		// to stdout on success (the `cd -` form).
		target, printResult, err := resolveOperand(args, callCtx)
		if err != nil {
			callCtx.Errf("cd: %s\n", err)
			return builtins.Result{Code: 1}
		}

		// bash 5.2 treats an empty target as a *no-op cd to the
		// current dir*: pwd is unchanged but $PWD and $OLDPWD are
		// still refreshed so subsequent navigation (including cd -)
		// sees consistent state. We mirror that by redirecting the
		// empty target to cwd and letting ChangeDir update the
		// env vars normally. The printResult branch below still
		// prints just a newline for the `cd -` form so the visible
		// output also matches bash.
		emptyTarget := target == ""

		// Convert the operand to an absolute candidate path. Relative
		// paths are joined with the current working directory.
		//
		// We deliberately avoid filepath.Join: it Cleans the result,
		// lexically collapsing ".." before resolvePath gets a chance
		// to validate intermediate components. `cd no-such/..` must
		// fail (the prefix doesn't exist) — Join would silently
		// reduce it to cwd. Concatenate raw and let resolvePath walk.
		cwd := callCtx.WorkDir()
		absTarget := target
		if emptyTarget {
			absTarget = cwd
		}
		if !filepath.IsAbs(absTarget) {
			if cwd == "" {
				callCtx.Errf("cd: cannot resolve relative path: no working directory\n")
				return builtins.Result{Code: 1}
			}
			sep := string(filepath.Separator)
			if strings.HasSuffix(cwd, sep) {
				absTarget = cwd + absTarget
			} else {
				absTarget = cwd + sep + absTarget
			}
		}

		physical := physicalFlag.pos > logicalFlag.pos
		resolved, err := resolvePath(ctx, callCtx, absTarget, physical)
		if err != nil {
			callCtx.Errf("cd: %s: %s\n", target, formatErr(callCtx, err))
			return builtins.Result{Code: 1}
		}

		if err := callCtx.ChangeDir(resolved); err != nil {
			callCtx.Errf("cd: %s: %s\n", target, formatErr(callCtx, err))
			return builtins.Result{Code: 1}
		}

		if printResult {
			if emptyTarget {
				// bash prints just a newline for `OLDPWD= cd -`.
				callCtx.Out("\n")
			} else {
				callCtx.Outf("%s\n", resolved)
			}
		}
		return builtins.Result{}
	}
}

// resolveOperand inspects the parsed positional arguments and returns the
// target path string. printResult is true when the caller must print the
// resolved path on success (the `cd -` form). An error is returned only
// when the required env var (HOME / OLDPWD) is *unset*.
//
// bash distinguishes "unset" from "set to empty" for both HOME and
// OLDPWD: an empty value is accepted as a valid (no-op) target, only an
// unset variable is rejected. The caller short-circuits on target == ""
// so we do not synthesise an "empty path" sandbox call.
func resolveOperand(args []string, callCtx *builtins.CallContext) (target string, printResult bool, err error) {
	if len(args) == 0 {
		// No-arg form: change to $HOME.
		home, ok := lookupEnv(callCtx, "HOME")
		if !ok {
			return "", false, errors.New("HOME not set")
		}
		return home, false, nil
	}

	operand := args[0]
	if operand == "-" {
		oldpwd, ok := lookupEnv(callCtx, "OLDPWD")
		if !ok {
			return "", false, errors.New("OLDPWD not set")
		}
		return oldpwd, true, nil
	}
	return operand, false, nil
}

// lookupEnv reads name from the runner's environment via the LookupEnvVar
// callback. When the callback is unavailable (e.g. an exotic test
// configuration), it returns ("", false) — matching the unset case so
// callers do not need to handle a third state.
func lookupEnv(callCtx *builtins.CallContext, name string) (string, bool) {
	if callCtx.LookupEnvVar == nil {
		return "", false
	}
	return callCtx.LookupEnvVar(name)
}

// resolvePath canonicalises absPath to the form that ChangeDir expects
// using bash's per-component validation rules.
//
// Both -L and -P walk the path component-by-component:
//   - "." is skipped.
//   - ".." is processed only after verifying the prefix accumulated so
//     far is an existing directory (POSIX-required for cd; otherwise
//     `cd no-such/..` or `cd file/..` would silently succeed via
//     lexical filepath.Clean).
//   - Named components are joined onto the accumulating prefix.
//
// For -P (physical) named components are additionally expanded through
// the sandbox's symlink-safe Lstat / Readlink callbacks before ".." is
// processed, capped at maxSymlinkHops to defeat cycles. For -L
// (logical) symlinks are preserved in the result.
//
// Note: unlike pwd -P, cd does not apply CanonicalizeRootPrefix to the
// resolved path. cd needs the working directory in the configured-root
// form (e.g. /tmp/link) so subsequent sandbox operations resolve under
// the same root.
func resolvePath(ctx context.Context, callCtx *builtins.CallContext, absPath string, physical bool) (string, error) {
	if !filepath.IsAbs(absPath) {
		return "", fmt.Errorf("not an absolute path: %s", absPath)
	}
	if callCtx.StatFile == nil || callCtx.LstatFile == nil || callCtx.ReadlinkFile == nil {
		// Sandbox-less fallback: best-effort lexical cleaning. The
		// outer ChangeDir validation still applies, so a missing or
		// non-directory final target still errors.
		return filepath.Clean(absPath), nil
	}

	// On Windows the user may write `cd a/../b` (forward slashes); the
	// component splitter below only recognises filepath.Separator. Normalise
	// '/' to the OS separator without invoking filepath.Clean — Clean would
	// collapse ".." lexically and defeat the per-component validation that
	// is the whole point of this walker.
	absPath = filepath.FromSlash(absPath)

	out := rootPrefix(absPath)
	rest := strings.TrimPrefix(absPath, out)

	hops := 0
	for rest != "" {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		// Consume any leading separator(s) before extracting the next
		// component. filepath.Clean collapses runs of separators in
		// the input, but expanded symlink targets that we splice back
		// into rest can reintroduce them.
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
			// POSIX/bash: validate the prefix accumulated so far is
			// an existing directory before popping. Without this,
			// `cd no-such/..` and `cd file/..` would silently
			// succeed (they don't in bash). Use StatFile so the
			// symlink target — not the link itself — is checked,
			// matching bash's chdir-via-resolved-path semantics.
			//
			// Above-sandbox paths fail Stat with ErrPermission;
			// treat them as opaque pass-throughs so a `cd ..` at
			// the sandbox root or a path that briefly traverses
			// above-root components still works. The final
			// ChangeDir will reject any resulting outside-sandbox
			// target.
			info, err := callCtx.StatFile(ctx, out)
			if err != nil {
				if errors.Is(err, iofs.ErrPermission) {
					out = parentDir(out)
					continue
				}
				return "", err
			}
			if !info.IsDir() {
				return "", &os.PathError{Op: "chdir", Path: out, Err: errNotDirectory}
			}
			out = parentDir(out)
			continue
		}

		candidate := joinPath(out, comp)

		if !physical {
			// -L: keep symlinks intact; don't stat per-append. The
			// final ChangeDir's Stat will catch a missing or
			// non-dir leaf.
			out = candidate
			continue
		}

		// -P: resolve symlinks before they become part of `out` so
		// a subsequent ".." processes the resolved target.
		info, err := callCtx.LstatFile(ctx, candidate)
		if err != nil {
			// Above-sandbox paths legitimately fail Lstat (e.g. the
			// runner's cwd is /sandbox/... so the walker traverses
			// /private, /var, /folders, ... before re-entering the
			// sandbox). Treat them as opaque pass-throughs so cd -P
			// to a sandbox-internal target still resolves. Missing
			// in-sandbox components surface as ErrNotExist instead
			// and bubble up as a hard error, matching bash.
			if errors.Is(err, iofs.ErrPermission) {
				out = candidate
				continue
			}
			return "", err
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
			return "", err
		}

		if filepath.IsAbs(target) {
			cleanedTarget := filepath.Clean(target)
			// Container-style sandboxes mount the host filesystem at
			// a prefix (e.g. /mnt/host). Symlink targets stored on
			// disk often refer to host-absolute paths without that
			// prefix, so apply the prefix when set.
			if callCtx.HostPrefix != nil {
				if hp := callCtx.HostPrefix(); hp != "" && !strings.HasPrefix(cleanedTarget, hp+string(filepath.Separator)) && cleanedTarget != hp {
					cleanedTarget = filepath.Join(hp, cleanedTarget)
				}
			}
			out = rootPrefix(cleanedTarget)
			rest = strings.TrimPrefix(cleanedTarget, out) + rest
		} else {
			// filepath.Clean normalises the relative target —
			// collapsing "." segments and converting forward
			// slashes to the host separator on Windows.
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
// root. If dir is already at the root, parentDir returns dir unchanged
// (matching POSIX: at "/", ".." stays at "/").
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

// formatErr renders err as a short, POSIX-style message. The cd builtin
// always prefixes its output with `cd: <operand>:`, so we strip the
// `op path:` prefix that os.PathError adds — otherwise the user sees
// `cd: foo: statat foo: no such file or directory`, with the operand
// quoted twice. Errors that are not *os.PathError fall through to
// PortableErr (when available) or Error().
func formatErr(callCtx *builtins.CallContext, err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) && pe.Err != nil {
		return pe.Err.Error()
	}
	if callCtx.PortableErr != nil {
		if msg := callCtx.PortableErr(err); msg != "" {
			return msg
		}
	}
	return err.Error()
}

// boolSeqFlag is a pflag.Value implementation for -L/-P. Two boolSeqFlag
// values share a *seq counter; each call to Set increments the counter
// and records the new value in pos. After parsing, comparing pos fields
// reveals which flag appeared last on the command line.
//
// Set also rejects explicit values: cd's mode flags are bare boolean
// switches and `cd --physical=false` should fail like bash rather than
// silently flip the mode.
type boolSeqFlag struct {
	seq *int
	pos int
}

func newBoolSeqFlag(seq *int) *boolSeqFlag {
	return &boolSeqFlag{seq: seq}
}

func (f *boolSeqFlag) String() string { return "false" }

func (f *boolSeqFlag) Set(s string) error {
	if s != boolSeqSentinel {
		return errBoolSeqValue
	}
	*f.seq++
	f.pos = *f.seq
	return nil
}

func (f *boolSeqFlag) Type() string { return "bool" }

func printHelp(callCtx *builtins.CallContext) {
	callCtx.Out(`Usage: cd [-LP] [-|DIRECTORY]
Change the shell's current working directory.

With no DIRECTORY, change to $HOME. With '-', change to $OLDPWD
and print the new directory.

  -L, --logical   treat .. lexically; preserve symbolic links
                  in the result (default)
  -P, --physical  resolve symbolic links before treating ..
  -h, --help      display this help and exit
`)
}
