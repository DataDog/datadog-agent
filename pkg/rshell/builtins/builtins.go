// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package builtins defines restricted shell commands and their capabilities.
package builtins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/spf13/pflag"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/flagparser"
)

// ErrVarStorageExceeded is returned by CallContext.SetVar when the
// assignment would push the runner's total variable storage past its
// cap. This is a script-aborting condition: AST-level assignments
// treat it the same way (see interp/vars.go), so state-mutating
// builtins should propagate it via Result.Exiting=true rather than
// continuing with status 1, matching bash's resource-cap DoS guard.
var ErrVarStorageExceeded = errors.New("variable storage limit exceeded")

// MaxFileRemovalsPerRun is the cumulative number of files that may be removed
// through CallContext.Remove across an entire Runner.Run call, including every
// loop iteration, subshell, and pipeline stage. It exists because the
// per-invocation cap in the rm builtin bounds only a single mistaken glob:
// `for f in *; do rm "$f"; done` and `find … | xargs -n1 rm` each drive an
// unbounded number of single-file invocations past it. The run-wide budget is
// the only limit that matches the threat model of an AI agent writing ordinary
// loop idioms.
//
// The value is deliberately much larger than the per-invocation cap: a run-wide
// budget must not break real remediation scripts (rotating a few dozen stale
// log files is a legitimate cleanup), while still bounding an unattended run to
// an amount of damage an operator can reason about and recover from. It is a
// fixed constant rather than a RunnerOption on purpose — one number that every
// deployment shares is harder to misconfigure than a knob, and no caller has
// yet shown a bulk-cleanup need that justifies the extra configuration surface.
// Both the number and the configurability question are open for maintainer
// sign-off.
const MaxFileRemovalsPerRun = 100

// ErrRemoveBudgetExceeded is returned by CallContext.Remove once the run-wide
// MaxFileRemovalsPerRun budget is exhausted. The file is not removed. Builtins
// should stop processing further operands when they see it, since every
// subsequent removal in the same run will fail the same way.
var ErrRemoveBudgetExceeded = errors.New("run-wide file removal budget exceeded")

// ErrWriteOutcomeUnknown is returned (wrapped) by CallContext.WriteRegularFile
// when ctx becomes done while the underlying write is still running in an
// abandoned background goroutine, rather than a plain ctx.Err() from a
// check that ran before any byte was written, or a genuine I/O error from a
// completed attempt. It specifically signals that the target file's
// descriptor may still be actively written to for an indeterminate time
// after the call returns, so a caller must not perform a second, concurrent
// write against the same path (e.g. a restore-on-failure attempt) until it
// can otherwise establish the abandoned write has actually stopped — doing
// so anyway would race the abandoned write on the same inode, and whichever
// write lands last wins, silently corrupting the file regardless of which
// caller's content "should" have won. mutated is still conservatively
// reported as true alongside this error, since a caller that has some
// other, safe way to eventually retry a restore (e.g. after re-verifying
// the file's identity once it can establish the abandoned write has
// stopped) still needs to know a restore may be warranted at all — mutated
// only distinguishes "nothing to restore" from "something might need
// restoring"; this error additionally distinguishes "safe to restore now"
// from "not safe to restore yet".
var ErrWriteOutcomeUnknown = errors.New("write outcome unknown: a background write may still be in progress")

// FlagSet is a type alias for pflag.FlagSet. Command files receive a *FlagSet
// from the framework without needing to import pflag directly (the builtins
// package is always allowed by the import allowlist).
type FlagSet = pflag.FlagSet

// Flag is a type alias for pflag.Flag, exposed so command files can use
// FlagSet.Visit without importing pflag directly.
type Flag = pflag.Flag

// HandlerFunc is the bound handler called by the framework after flags are
// parsed. args contains only the positional (non-flag) arguments.
type HandlerFunc func(ctx context.Context, callCtx *CallContext, args []string) Result

// AllowedPathAccess is the configured filesystem access level for one
// AllowedPaths root.
type AllowedPathAccess string

const (
	AllowedPathReadOnly  AllowedPathAccess = "read-only"
	AllowedPathReadWrite AllowedPathAccess = "read-write"
)

// AllowedPath describes one resolved AllowedPaths sandbox root.
type AllowedPath struct {
	Path   string
	Access AllowedPathAccess
}

const (
	// SystemdJournaldService is the exact service name used for journal-wide
	// operations such as kernel log reads, disk usage, rotation, and vacuuming.
	SystemdJournaldService = "systemd-journald.service"
)

// SystemdOperation is one unit action that must be authorized before a
// builtin interacts with systemd.
type SystemdOperation struct {
	Service string
	Action  SystemServiceAction
}

// Command pairs a builtin name with its flag-declaring factory. MakeFlags
// registers any flags on the provided FlagSet and returns the bound handler.
// Commands that accept no flags may ignore fs via NoFlags.
type Command struct {
	Name        string
	Description string
	Help        string
	MakeFlags   func(*FlagSet) HandlerFunc

	// NormalizeArgs, if non-nil, rewrites raw argument slices before pflag
	// parsing. This allows commands to support legacy flag syntax that pflag
	// cannot handle natively (e.g. head/tail -5 → -n 5).
	NormalizeArgs func(args []string) []string

	// RemediationOnly marks a builtin as only available in remediation mode.
	// The interpreter refuses to dispatch such a command — before flag
	// parsing, so --help is refused too — when the shell is in read-only
	// mode, and the help builtin moves it to the disabled list. Builtins
	// keep their own equivalent check as defence in depth; the dispatch
	// gate is what makes the flag load-bearing for future builtins.
	RemediationOnly bool

	// RemediationDeniedMessage overrides the stderr text written by the
	// dispatch-level read-only refusal. It must end with a newline. When
	// empty, DefaultRemediationDeniedMessage is used. Only meaningful
	// together with RemediationOnly.
	RemediationDeniedMessage string
}

// DefaultRemediationDeniedMessage returns the stderr text written when a
// RemediationOnly builtin is invoked in read-only mode and the command does
// not set RemediationDeniedMessage.
func DefaultRemediationDeniedMessage(name string) string {
	return name + ": remediation mode required\n"
}

// NoFlags wraps a HandlerFunc in the MakeFlags format for commands that
// declare no flags.
func NoFlags(fn HandlerFunc) func(*FlagSet) HandlerFunc {
	return func(_ *FlagSet) HandlerFunc { return fn }
}

// Register adds the Command to the builtin registry. For each invocation the
// framework creates a fresh *FlagSet, passes it to MakeFlags so the command
// can register its flags, parses the raw args, writes any error to stderr
// (exit 1), and then calls the bound handler with positional args only.
//
// If MakeFlags registers no flags (e.g. via NoFlags), the framework skips
// parsing entirely and passes all raw args to the handler unchanged. This
// lets commands like echo treat flag-shaped literals (e.g. -n) correctly.
func (c Command) Register() {
	name := c.Name
	factory := c.MakeFlags
	if _, exists := featureByName[name]; exists {
		panic("builtin name conflicts with rshell feature: " + name)
	}

	// Probe whether the command registers any flags so we can record it
	// in metadata (used by tests to enforce help-consistency invariants).
	probe := pflag.NewFlagSet(name, pflag.ContinueOnError)
	probe.SetOutput(io.Discard)
	factory(probe)
	hasFlags := probe.HasFlags()

	denied := c.RemediationDeniedMessage
	if c.RemediationOnly && denied == "" {
		denied = DefaultRemediationDeniedMessage(name)
	}
	metaRegistry[name] = CommandMeta{
		Name:                     name,
		Description:              c.Description,
		Help:                     c.Help,
		HasFlags:                 hasFlags,
		RemediationOnly:          c.RemediationOnly,
		RemediationDeniedMessage: denied,
	}
	commandRegistry[name] = c
	addToRegistry(name, func(ctx context.Context, callCtx *CallContext, args []string) Result {
		handler, fs, operands, err := c.parseArgs(args)
		hasHelp := fs.Lookup("help") != nil
		if err != nil {
			callCtx.Errf("%s: %s\n", name, err)
			if hasHelp {
				callCtx.Errf("Try '%s --help' for more information.\n", name)
			}
			return Result{Code: 1}
		}
		return handler(ctx, callCtx, operands)
	})
}

// CallContext provides the capabilities available to builtin commands.
// It is created by the Runner for each builtin invocation.
type CallContext struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	// InLoop is true when the builtin runs inside a for loop.
	InLoop bool

	// LastExitCode is the exit code from the previous command.
	LastExitCode uint8

	// Env iterates over the exported environment snapshot for this command. It
	// includes caller-provided Env values and exported shell variables, but not
	// unexported variables or the host process environment unless supplied.
	Env func(func(name, value string) bool)

	// OpenFile opens a file within the shell's path restrictions.
	OpenFile func(ctx context.Context, path string, flags int, mode os.FileMode) (io.ReadWriteCloser, error)

	// OpenRegularFile opens an identity-verified regular file through AllowedPaths.
	OpenRegularFile func(ctx context.Context, path string) (io.ReadCloser, error)

	// ReadDir reads a directory within the shell's path restrictions.
	// Entries are returned sorted by name. Used by builtins like ls
	// that need deterministic sorted output.
	ReadDir func(ctx context.Context, path string) ([]fs.DirEntry, error)

	// OpenDir opens a directory within the shell's path restrictions for
	// incremental reading via ReadDir(n). Caller must close the handle.
	OpenDir func(ctx context.Context, path string) (fs.ReadDirFile, error)

	// IsDirEmpty checks whether a directory is empty by reading at most
	// one entry. More efficient than reading all entries.
	IsDirEmpty func(ctx context.Context, path string) (bool, error)

	// ReadDirLimited reads directory entries, skipping the first offset entries
	// and returning up to maxRead entries sorted by name within the read window.
	// Returns (entries, truncated, error). When truncated is true, the directory
	// contained more entries beyond the returned set.
	ReadDirLimited func(ctx context.Context, path string, offset, maxRead int) ([]fs.DirEntry, bool, error)

	// StatFile returns file info within the shell's path restrictions (follows symlinks).
	StatFile func(ctx context.Context, path string) (fs.FileInfo, error)

	// FileSystemStat returns filesystem-wide metadata for the filesystem
	// containing path. The path is resolved within the shell's path
	// restrictions and symlinks are followed.
	FileSystemStat func(ctx context.Context, path string) (FileSystemInfo, error)

	// LstatFile returns file info within the shell's path restrictions (does not follow symlinks).
	LstatFile func(ctx context.Context, path string) (fs.FileInfo, error)

	// ReadlinkFile returns the destination of a symbolic link within the
	// shell's path restrictions.
	ReadlinkFile func(ctx context.Context, path string) (string, error)

	// AccessFile checks whether the file at path is accessible with the given mode
	// within the shell's path restrictions. Mode: 0x04=read, 0x02=write, 0x01=execute.
	AccessFile func(ctx context.Context, path string, mode uint32) error

	// Truncate sets the size of the file at path within the shell's path
	// restrictions. When create is true, a missing file is created (mode
	// 0666 & ~umask); when create is false, a missing file returns
	// os.ErrNotExist. Negative sizes are rejected. Only available in
	// remediation mode; nil otherwise.
	Truncate func(ctx context.Context, path string, size int64, create bool) error

	// TruncateToZeroIfAtLeast truncates path to zero bytes within the shell's
	// path restrictions when its pre-truncation size is at least minSize.
	// When dryRun is true, it performs the same write-target validation and
	// eligibility check without mutating the file. The size check and truncate
	// share one fd to avoid path-swap races. Missing files are not created.
	// Only available in remediation mode; nil otherwise.
	TruncateToZeroIfAtLeast func(ctx context.Context, path string, minSize int64, dryRun bool) (sizeBefore int64, truncated bool, err error)

	// Remove deletes the file at path within the shell's path restrictions.
	// Directories are always rejected with an error; any other non-directory
	// entry (regular file, symlink, FIFO, socket, device node) may be
	// removed. A symlink argument removes the link itself, not its referent.
	// Only available in remediation mode; nil otherwise.
	//
	// Removals are charged against a cumulative per-run budget of
	// MaxFileRemovalsPerRun files, shared across every invocation, loop
	// iteration, subshell, and pipeline stage in one Runner.Run call. Once the
	// budget is exhausted, Remove returns ErrRemoveBudgetExceeded without
	// touching the file. Failed removals are not charged.
	Remove func(ctx context.Context, path string) error

	// WriteRegularFile atomically validates that path is (and remains) a
	// regular file and overwrites its entire content with data, sharing a
	// single file descriptor across the type check and the write so nothing
	// can be swapped in between validation and the destructive write (e.g. a
	// FIFO or device substituted for the checked regular file). It never
	// creates a missing file. Only available in remediation mode; nil
	// otherwise.
	//
	// expectedIdentity, if non-nil, must be an fs.FileInfo obtained from a
	// prior Stat of the same descriptor the caller read data's replacement
	// content from (or, for a restore call, the descriptor the original
	// content came from). The write is rejected if the file opened for
	// writing is not the same file (compared via os.SameFile) — this closes
	// the identity gap that the single-descriptor type check alone cannot:
	// that check proves the descriptor opened for writing is *a* regular
	// file, not that it is the *same* regular file the caller's data is
	// derived from. Pass nil to skip the identity check (e.g. when the
	// caller has no prior read to pin against).
	//
	// The returned bool reports whether this call actually mutated the
	// file's on-disk content before returning, regardless of whether it
	// also returned an error — letting a caller distinguish "failed after
	// already changing some bytes, so a best-effort restore is warranted"
	// from "failed (e.g. cancelled) before touching the file at all, so it
	// is untouched and no restore should be attempted".
	WriteRegularFile func(ctx context.Context, path string, data []byte, expectedIdentity fs.FileInfo) (mutated bool, err error)

	// RemediationMode reports whether the shell is running in remediation mode.
	// When false (read-only mode), write-capable builtins such as truncate are
	// not available. Used by the help builtin to partition commands correctly.
	RemediationMode bool

	// PortableErr normalizes an OS error to a POSIX-style message.
	PortableErr func(err error) string

	// Now is the time captured at the start of each Run() call. Builtins
	// should use this instead of calling time.Now() directly, so the time
	// source is consistent across all commands in a single run.
	//
	// Note: this means all builtins within one Run() share the same reference
	// time, whereas bash evaluates each command against its own invocation
	// time. This is an intentional trade-off for consistency within a script
	// run.
	//
	// Run() always sets this before dispatching any builtin; Reset() clears
	// it, so it is always re-set by the next Run() call. The zero value
	// (time.Time{}) is reserved as the unset sentinel; callers constructing
	// CallContext directly (e.g. in tests) must set this to a non-zero value
	// before invoking builtins that use time predicates (find -mmin/-mtime,
	// ls -l).
	Now time.Time

	// FileIdentity extracts canonical file identity from FileInfo.
	// On Unix: dev+inode from Stat_t. On Windows: volume serial + file index
	// via GetFileInformationByHandle. The path parameter is needed on Windows
	// where FileInfo.Sys() lacks identity fields; Unix ignores it.
	FileIdentity func(path string, info fs.FileInfo) (FileID, bool)

	// CommandAllowed reports whether a command name is permitted under the
	// current shell policy. Used by the help builtin to list only executable
	// commands.
	CommandAllowed func(name string) bool

	// ElevatableCommandsList returns the sorted command names that may be
	// prefixed with sudo under the current shell policy. The returned slice is
	// a defensive copy. Used by the help builtin to surface selective elevation.
	ElevatableCommandsList func() []string

	// AuthorizeSystemd reports whether every operation may be performed under
	// the current shell policy. Implementations must authorize the complete
	// list before a builtin performs any operation so compound requests cannot
	// partially execute.
	AuthorizeSystemd func(operations ...SystemdOperation) error

	// AuthorizeSystemServices is the deprecated unit-only authorization
	// capability retained for compatibility with callers built against the
	// original service allowlist API.
	AuthorizeSystemServices func(action SystemServiceAction, services ...string) error

	// ReadableSystemServices returns the exact, sorted unit selectors granted
	// the read action. The returned slice is a defensive copy. Restricted
	// enumeration commands use this capability instead of listing every unit on
	// the configured systemd target.
	ReadableSystemServices func() []string

	// AllowedSystemServicesList returns the effective, exact unit/action grant
	// pairs sorted by unit and canonical action order. The returned slice is a
	// defensive copy. Used by the help builtin to surface the active systemd
	// capability policy without exposing a mutable authorization map.
	AllowedSystemServicesList func() []SystemdOperation

	// AllowedPathsList returns the resolved absolute paths and configured
	// access modes of the AllowedPaths sandbox roots. An empty/nil slice means
	// no allowed paths are configured, which blocks all filesystem access.
	// Used by the help builtin to surface the active sandbox roots.
	AllowedPathsList func() []AllowedPath

	// WorkDir returns the shell's current working directory (absolute path).
	// Used by builtins that need to compute absolute paths for sub-operations.
	WorkDir func() string

	// HostPrefix returns the configured host-mount prefix used by
	// container-style sandboxes to translate host-absolute paths
	// (e.g. /var/log/pods/...) into the prefixed paths the sandbox can
	// open (e.g. /mnt/host/var/log/pods/...). Returns "" when no prefix
	// is configured. Builtins that resolve absolute symlink targets
	// (e.g. pwd -P) use this to keep their output consistent with what
	// the sandbox itself accepts.
	HostPrefix func() string

	// CanonicalizeRootPrefix translates a configured AllowedPaths root
	// prefix in absPath to that root's canonical (symlink-resolved)
	// form. Used by `pwd -P` so that when the sandbox root is itself a
	// symlink (e.g. /tmp/link -> /tmp/real), the printed path reflects
	// the resolution that os.Root has already followed implicitly. If
	// absPath is outside every root or the matching root is not a
	// symlink, the input is returned unchanged.
	CanonicalizeRootPrefix func(absPath string) string

	// ChangeDir mutates the shell's working directory. The supplied path
	// must be absolute. Implementations validate that the target exists,
	// is a directory, and lies inside AllowedPaths; on any failure the
	// previous working directory is preserved and an error is returned.
	// On success, $OLDPWD is set to the previous directory and $PWD is
	// set to absDir. Used exclusively by the cd builtin.
	ChangeDir func(absDir string) error

	// LookupEnvVar reads an environment variable from the shell's
	// overlay environment. Returns (value, true) if the variable is
	// set, ("", false) otherwise. The cd builtin uses this to resolve
	// $HOME (no-arg form) and $OLDPWD (the `cd -` form) without
	// requiring a full WriteEnviron handle on every CallContext.
	LookupEnvVar func(name string) (string, bool)

	// RunCommand executes a builtin command within the shell's sandbox.
	// dir overrides the working directory for path resolution.
	// Returns the command's exit code.
	RunCommand func(ctx context.Context, dir string, name string, args []string) (uint8, error)

	// RunCommandWithStdin is like RunCommand but lets the caller supply a
	// stdin reader for the child. Used by xargs to give children empty
	// stdin (matching POSIX behavior of redirecting child stdin from
	// /dev/null) while still reading items from the parent's stdin itself.
	// If nil, callers should fall back to RunCommand.
	RunCommandWithStdin func(ctx context.Context, dir string, name string, args []string, stdin io.Reader) (uint8, error)

	// SetVar assigns a value to a shell variable in the calling shell's
	// scope. Returns an error if the value exceeds the per-variable size
	// limit or if the total variable-storage cap would be exceeded.
	// Used by builtins that mutate parent-shell state, such as read.
	SetVar func(name, value string) error

	// GetVar returns the value of a shell variable. The bool reports
	// whether the variable was set; an unset variable returns ("", false).
	// Used by builtins that need to consult shell state, such as read
	// reading IFS for field-splitting.
	GetVar func(name string) (value string, ok bool)

	// Proc provides access to the proc filesystem for the ps and lsof
	// builtins. The path is fixed at construction time and cannot be
	// overridden by callers.
	Proc *ProcProvider

	// Systemd contains structured backends for systemd-aware builtins. Target
	// paths and transports are fixed by trusted runner configuration.
	Systemd *SystemdServices
}

// Out writes a string to stdout.
func (c *CallContext) Out(s string) {
	io.WriteString(c.Stdout, s)
}

// Outf writes a formatted string to stdout.
func (c *CallContext) Outf(format string, a ...any) {
	fmt.Fprintf(c.Stdout, format, a...)
}

// Errf writes a formatted string to stderr.
func (c *CallContext) Errf(format string, a ...any) {
	fmt.Fprintf(c.Stderr, format, a...)
}

// SafeOperand escapes control characters in s — newlines, tabs, ESC, and
// other non-printable bytes — so the result can be interpolated into a
// single-quoted error message (e.g. "cmd: extra operand '%s'") without
// letting a crafted operand forge additional diagnostic lines or inject
// terminal/log control sequences into stderr. It also escapes Unicode line/
// paragraph separators (U+2028, U+2029) and format characters (e.g. the
// bidi override U+202E), since unicode.IsControl doesn't cover those but
// Unicode-aware log viewers still act on them to split or visually reorder
// the diagnostic. It intentionally does not escape the single quote itself
// or otherwise shell-quote the value; it only neutralizes runes that are
// dangerous in a raw stderr stream, mirroring the "literal" tier of GNU
// coreutils' quotearg rather than its full shell-quoting modes.
//
// Escaping a literal backslash makes this the wrong choice for
// already-formatted diagnostic text that may legitimately contain one
// (e.g. a Windows path separator) — use SafeMessage for that case instead.
func SafeOperand(s string) string {
	return escapeUnsafeRunes(s, true)
}

// SafeMessage escapes the same control characters as SafeOperand —
// newlines, tabs, ESC, and other non-printable or Unicode line/format
// characters — but leaves a literal backslash untouched.
//
// Use this (instead of SafeOperand) when the string being escaped is
// already-formatted diagnostic text that may legitimately contain a
// backslash for a reason unrelated to escaping — most notably a Windows
// path separator embedded in an *os.PathError's Error() string. Escaping
// backslash there would double every separator and corrupt the message
// (e.g. "missing_dir\out.txt" becoming "missing_dir\\out.txt"). For a raw,
// unformatted user-supplied operand (a shell-script argument, a filename),
// use SafeOperand instead: there, a literal backslash could itself be used
// to build a forged escape sequence once the result is re-displayed, so it
// must be escaped too.
func SafeMessage(s string) string {
	return escapeUnsafeRunes(s, false)
}

// escapeUnsafeRunes is the shared implementation behind SafeOperand and
// SafeMessage. escapeBackslash controls whether a literal backslash is
// itself escaped (SafeOperand: yes; SafeMessage: no, see SafeMessage's doc
// comment for why).
func escapeUnsafeRunes(s string, escapeBackslash bool) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\' && escapeBackslash:
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf):
			if r > 0xff {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				fmt.Fprintf(&b, `\x%02x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsBrokenPipe reports whether err is a broken-pipe error, which occurs
// when writing to a pipe whose read end has been closed. In bash this
// triggers SIGPIPE which silently terminates the writer; builtins should
// use this to suppress error messages on pipe closure.
//
// On Unix this is syscall.EPIPE. On Windows, os.Pipe's write side reports
// ERROR_BROKEN_PIPE (109) or ERROR_NO_DATA (232) instead — there is no
// EPIPE errno on that platform. Those two numeric codes are only checked
// on Windows (via isBrokenPipePlatform, in the *_windows.go/*_other.go
// pair): checking them unconditionally on every platform would be wrong,
// since errno 109 is ETOOMANYREFS on Linux — a real, unrelated error that
// must not be misreported as a closed pipe.
func IsBrokenPipe(err error) bool {
	return err != nil && (errors.Is(err, syscall.EPIPE) || isBrokenPipePlatform(err))
}

// FileID is a comparable file identity for cycle detection.
// On Unix: device + inode. On Windows: volume serial + file index.
// Used as map key for visited-set tracking.
type FileID struct {
	Dev uint64
	Ino uint64
}

// FileSystemInfo is the normalized subset of filesystem metadata exposed to
// builtins such as stat. Availability flags distinguish unsupported values
// from legitimate zero counts.
type FileSystemInfo struct {
	ID          uint64
	IDAvailable bool

	NameMax          uint64
	NameMaxAvailable bool

	TypeID          uint64
	TypeIDAvailable bool
	TypeName        string

	IOBlockSize          uint64
	FundamentalBlockSize uint64
	Blocks               uint64
	BlocksFree           uint64
	BlocksAvailable      uint64

	Files          uint64
	FilesFree      uint64
	FilesAvailable bool
}

// Result captures the outcome of executing a builtin command.
type Result struct {
	// Code is the exit status code.
	Code uint8

	// Exiting signals that the shell should exit (set by the "exit" builtin).
	Exiting bool

	// BreakN > 0 means break out of N enclosing loops.
	BreakN int

	// ContinueN > 0 means continue from N enclosing loops.
	ContinueN int
}

var registry = map[string]HandlerFunc{}

// CommandMeta holds metadata about a registered builtin command.
type CommandMeta struct {
	Name            string
	Description     string
	Help            string
	HasFlags        bool // true when MakeFlags registers at least one flag
	RemediationOnly bool // true when the command requires remediation mode

	// RemediationDeniedMessage is the stderr text the interpreter writes
	// when the command is dispatched in read-only mode. Non-empty exactly
	// when RemediationOnly is true.
	RemediationDeniedMessage string
}

var metaRegistry = map[string]CommandMeta{}

func addToRegistry(name string, fn HandlerFunc) {
	if _, exists := registry[name]; exists {
		panic("builtin already registered: " + name)
	}
	// Defense-in-depth: Register() already checks this before calling
	// addToRegistry, so this branch is unreachable from current callers.
	// Kept to guard against future callers that bypass Register().
	if _, exists := featureByName[name]; exists {
		panic("builtin name conflicts with rshell feature: " + name)
	}
	registry[name] = fn
}

// Lookup returns the handler for a builtin command.
func Lookup(name string) (HandlerFunc, bool) {
	fn, ok := registry[name]
	return fn, ok
}

// Names returns a sorted list of all registered builtin command names.
func Names() []string {
	names := make([]string, 0, len(metaRegistry))
	for name := range metaRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Meta returns the metadata for a registered builtin command.
func Meta(name string) (CommandMeta, bool) {
	m, ok := metaRegistry[name]
	return m, ok
}

// NormalizeBareNumberArg rewrites legacy -N shorthand (e.g. -5) to -n N so
// that pflag can parse it. Only a bare -<digits> token in the first argument
// position is rewritten; -<digits> appearing later in the argument list is
// left unchanged (matching GNU head/tail behavior where the obsolete form is
// only accepted as the first option). Processing stops at "--".
//
// When the first argument is a value-taking flag (-n, -c, --lines, --bytes),
// the second argument is its value and must not be rewritten — even if it
// looks like -<digits> (e.g. "head -n -9223372036854775809").
//
// valueFlags lists the flags that consume the next argument as a value
// (e.g. []string{"-n", "-c", "--lines", "--bytes"}).
func NormalizeBareNumberArg(args []string, _ []string) []string {
	if len(args) == 0 {
		return args
	}
	a := args[0]
	if a == "--" {
		return args
	}
	if len(a) >= 2 && a[0] == '-' && a[1] >= '0' && a[1] <= '9' {
		out := make([]string, 0, len(args)+1)
		out = append(out, "-n", a[1:])
		out = append(out, args[1:]...)
		return out
	}
	return args
}

var commandRegistry = map[string]Command{}

// InspectArgs parses a registered command's flags without calling its handler.
// It uses exactly the same normalization and flag parsing as execution. The
// returned flag set and operands belong to the caller. Handler-level validation
// (such as parsing a sed program) is not performed.
func InspectArgs(name string, args []string) (*FlagSet, []string, error) {
	c, ok := commandRegistry[name]
	if !ok {
		return nil, nil, fmt.Errorf("unknown command %q", name)
	}
	// Normalizers and flag parsers may rewrite their input.
	_, fs, operands, err := c.parseArgs(append([]string(nil), args...))
	return fs, operands, err
}

func (c Command) parseArgs(args []string) (HandlerFunc, *FlagSet, []string, error) {
	fs := pflag.NewFlagSet(c.Name, pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	handler := c.MakeFlags(fs)
	if !fs.HasFlags() {
		return handler, fs, args, nil
	}
	if c.NormalizeArgs != nil {
		args = c.NormalizeArgs(args)
	}
	// Honor --help only when all preceding options parse successfully. Keep
	// this shared with execution so flags after --help have identical meaning.
	// Handler-time validation remains the handler's responsibility.
	if fs.Lookup("help") != nil {
		if idx, ok := flagparser.TrialHelpTrimIndex(c.Name, func(trial *pflag.FlagSet) {
			_ = c.MakeFlags(trial)
		}, args); ok {
			args = args[:idx+1]
		}
	}
	if err := fs.Parse(args); err != nil {
		return handler, fs, nil, fmt.Errorf("%s", flagparser.RewriteError(err, args))
	}
	return handler, fs, fs.Args(), nil
}
