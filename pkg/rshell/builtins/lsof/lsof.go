// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package lsof implements the lsof builtin command.
//
// lsof — list open files, with emphasis on deleted-but-still-open files
//
// Usage: lsof [-p PIDLIST] [-c NAME] [-u UIDLIST] [-a] [-h] [--help]
//
// Display open file descriptors across processes: numeric fds plus the
// cwd/rtd (root)/txt (executable) special descriptors. A file that has been
// unlinked while a process still holds it open is reported with a
// " (deleted)" suffix on NAME — the primary diagnostic this builtin exists
// for (e.g. explaining "disk full but du shows nothing").
//
// File descriptor enumeration is delegated to the internal procfd package,
// which reads /proc/<pid>/fd/* on Linux. The /proc read itself is exempt
// from the AllowedPaths sandbox for the same reason ss/ip route/df/free are:
// the paths are hardcoded, never derived from user input. However, unlike
// those commands, the resolved filesystem path shown in the NAME column IS
// checked against AllowedPaths: a path outside every configured root is
// replaced with "(restricted)" (or "(restricted) (deleted)"), and its
// DEVICE/SIZE/NODE columns are blanked alongside it, since those are
// per-file attributes tied to the same out-of-sandbox path (an exact byte
// count, device number, and inode would otherwise still fingerprint a
// specific restricted file even with NAME hidden). This is a deliberate
// divergence, made because NAME can point anywhere on the host filesystem,
// unlike the bounded kernel counters ss/df/free expose. With no
// AllowedPaths configured, every NAME is restricted (see
// builtins/help/help.go's "no allowed paths configured" message: an empty
// list means no filesystem paths are reachable, not "unrestricted").
// Sockets, pipes, and anonymous inodes are never real filesystem paths and
// are therefore never gated.
//
// Linux only; macOS and Windows exit 1 with "not supported on this
// platform" (see the free builtin for the same pattern and rationale).
//
// Accepted flags:
//
//	-p PIDLIST
//	    Select processes by comma- or space-separated PID list.
//
//	-c NAME
//	    Select processes whose command name has this literal prefix
//	    (no regex, no globbing).
//
//	-u UIDLIST
//	    Select processes by comma- or space-separated numeric UID list.
//	    Login-name resolution is out of scope (no /etc/passwd read);
//	    only numeric UIDs are accepted, matching the ps builtin's UID
//	    column.
//
//	-a
//	    AND the selection criteria above instead of the default OR.
//	    Has no effect when zero or one selector is given.
//
//	-h, --help
//	    Print usage to stdout and exit 0.
//
// Output columns:
//
//	COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
//
// SIZE/OFF reports the file's size in bytes, not its read/write offset
// (real lsof can show either, selected by flags this builtin does not
// implement); size is what serves the deleted-open-file diagnostic this
// tool targets.
//
// Rejected flags (intentionally not registered; rejected as unknown by
// pflag with exit 1): -i/-U/-s/-T/-n/-P (network detail, already covered
// by ss), +d/+D (unbounded directory-tree stat scans), +|-r (repeat mode),
// -D/-f/+f (persistent device-number cache files — builtins must not write
// files outside remediation capabilities), -g/-G/-v/-V/-w/-x/-X/-C/-o/-b/
// -e/-A/-k/-K/-z/-Z.
//
// Exit codes:
//
//	0  Success, including when zero files match with no selector given.
//	1  Unsupported platform, invalid flag value, extra operand, a
//	   selector matched zero files, or an OS error listing processes.
package lsof

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procfd"
)

// Cmd is the lsof builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "lsof",
	Description: "list open files",
	MakeFlags:   registerFlags,
}

// noArgSentinel is the NoOptDefVal used for -a/--and and --help so that
// explicit-value forms (--and=true) are rejected, matching GNU getopt's
// no-argument behaviour. See builtins/df/df.go's noArgBool for the full
// rationale: a NUL byte cannot appear in argv (execve rejects it), so any
// non-sentinel value passed to Set means the user wrote "=value" and must
// be refused.
const noArgSentinel = "\x00"

// noArgBool mirrors df.noArgBool/free.noArgBool. Duplicated locally (rather
// than shared) because it is a small, self-contained pflag.Value and the
// existing copies are unexported.
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

// registerNoArgBool installs a noArgBool flag and returns the *bool target.
func registerNoArgBool(fs *builtins.FlagSet, name, shorthand, usage string) *bool {
	target := new(bool)
	flag := fs.VarPF(&noArgBool{target: target}, name, shorthand, usage)
	flag.NoOptDefVal = noArgSentinel
	return target
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	pidList := fs.StringP("pid", "p", "", "select by PID list (comma or space separated)")
	cmdPrefix := fs.StringP("command", "c", "", "select by command-name prefix (literal, no regex)")
	uidList := fs.StringP("user", "u", "", "select by numeric UID list (comma or space separated)")
	and := registerNoArgBool(fs, "and", "a", "AND the selection criteria instead of OR")
	help := registerNoArgBool(fs, "help", "h", "print usage and exit")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}

		if len(args) > 0 {
			callCtx.Errf("lsof: extra operand '%s'\n", builtins.SafeOperand(args[0]))
			callCtx.Errf("Try 'lsof --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		hasPIDs := fs.Lookup("pid").Changed
		hasCmd := fs.Lookup("command").Changed
		hasUsers := fs.Lookup("user").Changed

		var pids []int
		if hasPIDs {
			var err error
			pids, err = parsePIDs(*pidList)
			if err != nil {
				callCtx.Errf("lsof: %v\n", err)
				return builtins.Result{Code: 1}
			}
		}

		var uids []string
		if hasUsers {
			var err error
			uids, err = parseUIDs(*uidList)
			if err != nil {
				callCtx.Errf("lsof: %v\n", err)
				return builtins.Result{Code: 1}
			}
		}

		// Argument validation above is platform-independent and runs
		// first, matching GNU tool conventions (bad usage is always an
		// error); only the actual data collection below is gated on
		// platform support.
		if runtime.GOOS != "linux" {
			callCtx.Errf("lsof: not supported on this platform\n")
			return builtins.Result{Code: 1}
		}

		sel := selectors{
			pids:      pids,
			hasPIDs:   hasPIDs,
			cmdPrefix: *cmdPrefix,
			hasCmd:    hasCmd,
			uids:      uids,
			hasUsers:  hasUsers,
			and:       *and,
		}

		// Narrow the proc scan to the requested PIDs when every active
		// selector requires PID membership anyway (pure -p, or -a
		// combined with -p): scanning only those PIDs is equivalent to
		// scanning everything and filtering, but cheaper. A plain OR
		// across -p and -c/-u cannot narrow this way, since a match
		// could come from -c/-u alone for a PID outside the list.
		var scanPIDs []int
		if hasPIDs && (sel.and || (!hasCmd && !hasUsers)) {
			scanPIDs = pids
		}

		files, err := callCtx.Proc.ListOpenFiles(ctx, scanPIDs, sel.processFilter())
		if err != nil {
			callCtx.Errf("lsof: %v\n", err)
			return builtins.Result{Code: 1}
		}

		if err := ctx.Err(); err != nil {
			return builtins.Result{Code: 1}
		}

		roots := gateRoots(callCtx)
		hostPrefix := ""
		if callCtx.HostPrefix != nil {
			hostPrefix = callCtx.HostPrefix()
		}

		sortOpenFiles(files)

		rows := make([]row, 0, len(files))
		for _, of := range files {
			if !sel.matches(of) {
				continue
			}
			rows = append(rows, toRow(of, roots, hostPrefix))
		}

		printRows(callCtx, rows)

		anySelector := hasPIDs || hasCmd || hasUsers
		if anySelector && len(rows) == 0 {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

// selectors holds the parsed -p/-c/-u values and the -a combination mode.
type selectors struct {
	pids      []int
	hasPIDs   bool
	cmdPrefix string
	hasCmd    bool
	uids      []string
	hasUsers  bool
	and       bool
}

// matches reports whether of satisfies the active selectors. With no
// selectors active, everything matches. With one or more active, they are
// OR-combined by default, AND-combined when -a is set.
func (s selectors) matches(of procfd.OpenFile) bool {
	if !s.hasPIDs && !s.hasCmd && !s.hasUsers {
		return true
	}

	var results []bool
	if s.hasPIDs {
		results = append(results, containsInt(s.pids, of.PID))
	}
	if s.hasCmd {
		results = append(results, strings.HasPrefix(of.Command, s.cmdPrefix))
	}
	if s.hasUsers {
		results = append(results, containsStr(s.uids, of.UID))
	}

	if s.and {
		for _, r := range results {
			if !r {
				return false
			}
		}
		return true
	}
	for _, r := range results {
		if r {
			return true
		}
	}
	return false
}

// processFilter adapts selectors into a procfd.ProcessFilter so that
// listProcess can reject a non-matching process before scanning its fd
// directory, rather than after. This is sound because matches depends only
// on of.PID, of.Command, and of.UID — never on any fd-specific field — so
// evaluating it once per process (before enumeration) is equivalent to the
// existing per-file filtering below. Returns nil (matching everything) when
// no selector is active, matching matches' own no-selector behaviour.
func (s selectors) processFilter() procfd.ProcessFilter {
	if !s.hasPIDs && !s.hasCmd && !s.hasUsers {
		return nil
	}
	return func(pid int, comm, uid string) bool {
		return s.matches(procfd.OpenFile{PID: pid, Command: comm, UID: uid})
	}
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// parsePIDs parses a comma- or whitespace-separated list of PIDs. Each PID
// must be a positive integer (> 0). Duplicated from builtins/ps/ps.go's
// parsePIDs rather than shared, matching the free/df noArgBool precedent
// for small, self-contained helpers.
func parsePIDs(s string) ([]int, error) {
	for _, seg := range strings.Split(s, ",") {
		if strings.TrimSpace(seg) == "" {
			return nil, fmt.Errorf("invalid PID list: %s", s)
		}
	}
	s = strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(s)
	pids := make([]int, 0, len(fields))
	seen := make(map[int]bool, len(fields))
	for _, f := range fields {
		pid, err := strconv.Atoi(f)
		if err != nil || pid <= 0 {
			return nil, fmt.Errorf("invalid PID: %s", f)
		}
		if !seen[pid] {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}
	if len(pids) == 0 {
		return nil, fmt.Errorf("invalid PID: %s", s)
	}
	return pids, nil
}

// parseUIDs parses a comma- or whitespace-separated list of numeric UIDs.
// Unlike PIDs, 0 (root) is a valid UID; negative values are rejected. Each
// UID is canonicalized to its plain decimal spelling (strconv.Atoi accepts
// a leading '+' and leading zeros, but /proc's Uid: field never does), so
// "+1000" and "01000" both compare equal to the "1000" a process's status
// file reports.
func parseUIDs(s string) ([]string, error) {
	for _, seg := range strings.Split(s, ",") {
		if strings.TrimSpace(seg) == "" {
			return nil, fmt.Errorf("invalid UID list: %s", s)
		}
	}
	s = strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(s)
	uids := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		uid, err := strconv.Atoi(f)
		if err != nil || uid < 0 {
			return nil, fmt.Errorf("invalid UID: %s", f)
		}
		canonical := strconv.Itoa(uid)
		if !seen[canonical] {
			seen[canonical] = true
			uids = append(uids, canonical)
		}
	}
	if len(uids) == 0 {
		return nil, fmt.Errorf("invalid UID: %s", s)
	}
	return uids, nil
}

// printHelp emits the help text to stdout (per RULES.md, help is not an
// error; exit 0 with output on stdout). Mirrors df's/free's NoOptDefVal-
// clearing dance so --help doesn't render a literal NUL byte for the
// no-argument flags.
func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: lsof [-p PIDLIST] [-c NAME] [-u UIDLIST] [-a] [-h] [--help]\n")
	callCtx.Out("List open files, including files still open after being deleted.\n\n")
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

// sortOpenFiles orders files deterministically by PID, then by descriptor
// (cwd, rtd, txt, then numeric fds ascending), so output does not depend on
// /proc directory iteration order.
func sortOpenFiles(files []procfd.OpenFile) {
	slices.SortFunc(files, func(a, b procfd.OpenFile) int {
		if a.PID != b.PID {
			return a.PID - b.PID
		}
		ra, na := fdRank(a.FD)
		rb, nb := fdRank(b.FD)
		if ra != rb {
			return ra - rb
		}
		return na - nb
	})
}

// fdRank returns a (category, numeric) sort key for a descriptor's FD
// column: cwd, rtd, and txt sort first (in that order), followed by
// numeric fds in ascending order.
func fdRank(fd string) (int, int) {
	switch fd {
	case "cwd":
		return 0, 0
	case "rtd":
		return 1, 0
	case "txt":
		return 2, 0
	}
	n, err := strconv.Atoi(fd)
	if err != nil {
		return 4, 0
	}
	return 3, n
}

// row is one rendered output line.
type row struct {
	command, pid, user, fd, typ, device, size, node, name string
}

func (r row) cells() []string {
	return []string{r.command, r.pid, r.user, r.fd, r.typ, r.device, r.size, r.node, r.name}
}

func toRow(of procfd.OpenFile, roots []gateRoot, hostPrefix string) row {
	device, size, node := of.Device, of.Size, of.Node
	if pathRestricted(of, roots, hostPrefix) {
		// DEVICE/SIZE/NODE are per-file attributes tied to the same
		// out-of-sandbox path as NAME (unlike ss/df/free's aggregate,
		// host-wide counters): an exact byte count, device number, and
		// inode would still let a caller fingerprint a specific
		// restricted file (e.g. /etc/shadow) even with NAME hidden. Blank
		// them alongside NAME rather than only gating the path string.
		device, size, node = "", "", ""
	}
	return row{
		command: sanitizeField(of.Command),
		pid:     strconv.Itoa(of.PID),
		user:    of.UID,
		fd:      of.FD,
		typ:     of.Type,
		device:  device,
		size:    size,
		node:    node,
		name:    sanitizeField(redactName(of, roots, hostPrefix)),
	}
}

// sanitizeField escapes control characters, other non-graphic runes, and
// invalid UTF-8 bytes in a field whose contents an unprivileged process can
// influence (the comm name, or a resolved filesystem path), so a crafted
// value (e.g. a process renamed via prctl(PR_SET_NAME) to embed a newline,
// or a file named with one) cannot inject extra lines or control sequences
// into lsof's output. Mirrors journalctl.go's appendEscaped/appendHexEscape
// convention: \n/\r/\t for those three controls, \xXX/\uXXXX/\UXXXXXXXX hex
// escapes for everything else non-graphic or invalid UTF-8.
func sanitizeField(value string) string {
	var out strings.Builder
	for value != "" {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			appendHexEscape(&out, 'x', uint32(value[0]), 2)
			value = value[1:]
			continue
		}
		switch r {
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			if !unicode.IsGraphic(r) {
				switch {
				case r <= 0xff:
					appendHexEscape(&out, 'x', uint32(r), 2)
				case r <= 0xffff:
					appendHexEscape(&out, 'u', uint32(r), 4)
				default:
					appendHexEscape(&out, 'U', uint32(r), 8)
				}
			} else {
				out.WriteString(value[:size])
			}
		}
		value = value[size:]
	}
	return out.String()
}

func appendHexEscape(output *strings.Builder, prefix byte, value uint32, width int) {
	const hex = "0123456789abcdef"
	output.WriteByte('\\')
	output.WriteByte(prefix)
	for shift := (width - 1) * 4; shift >= 0; shift -= 4 {
		output.WriteByte(hex[(value>>shift)&0x0f])
	}
}

// gateRoot pairs one AllowedPaths root's raw configured path with its
// canonical (symlink-resolved) alias, so pathWithinRoots can match a
// /proc-reported (kernel-canonical) path against either spelling. See
// gateRoots.
type gateRoot struct {
	raw       string
	canonical string
}

// gateRoots builds the gating roots for the current call: the shell's
// configured AllowedPaths, each paired with its canonical form via
// CanonicalizeRootPrefix (the same helper `pwd -P` uses to reflect a
// symlinked root's on-disk spelling). Without the canonical alias, a
// /proc/<pid>/fd readlink target — which the kernel always reports in
// canonical form — would fail to match a root that is itself a symlink.
func gateRoots(callCtx *builtins.CallContext) []gateRoot {
	if callCtx.AllowedPathsList == nil {
		return nil
	}
	paths := callCtx.AllowedPathsList()
	roots := make([]gateRoot, 0, len(paths))
	for _, p := range paths {
		canonical := p.Path
		if callCtx.CanonicalizeRootPrefix != nil {
			canonical = callCtx.CanonicalizeRootPrefix(p.Path)
		}
		roots = append(roots, gateRoot{raw: p.Path, canonical: canonical})
	}
	return roots
}

// redactName applies the AllowedPaths gate to a resolved filesystem path.
// Non-path targets (sockets, pipes, anonymous inodes — see
// procfd.OpenFile.IsPath) are never gated. memfds are gated like any other
// path despite never naming a real filesystem location: see the isRealPath
// comment in procfd_linux.go for why they cannot be safely exempted. A path
// outside every configured root is replaced with "(restricted)" so the
// deleted-file diagnostic signal survives (COMMAND/PID/USER/FD/TYPE are
// still shown) without disclosing where on the host filesystem the file
// lives. toRow additionally blanks DEVICE/SIZE/NODE on the same restricted
// rows (see pathRestricted), since those are per-file attributes tied to
// the same out-of-sandbox path and would otherwise still leak.
func redactName(of procfd.OpenFile, roots []gateRoot, hostPrefix string) string {
	if !of.IsPath {
		return of.Name
	}

	if pathRestricted(of, roots, hostPrefix) {
		if of.Deleted {
			return "(restricted) (deleted)"
		}
		return "(restricted)"
	}
	if of.Deleted {
		return of.Name + " (deleted)"
	}
	return of.Name
}

// pathRestricted reports whether of's resolved path falls outside every
// configured AllowedPaths root. Non-path targets (sockets, pipes,
// anonymous inodes) are never restricted, since they never name a
// filesystem location. Shared by redactName (gates NAME) and toRow (gates
// DEVICE/SIZE/NODE) so both apply the exact same boundary.
func pathRestricted(of procfd.OpenFile, roots []gateRoot, hostPrefix string) bool {
	if !of.IsPath {
		return false
	}

	// /proc/<pid>/fd targets can be host-absolute paths in container-style
	// sandboxes (e.g. /var/log/pods/...) that need the configured
	// HostPrefix applied before comparing against AllowedPaths, matching
	// pwd.go's/cd.go's resolveSymlinks handling of the same translation.
	cleaned := filepath.Clean(of.Name)
	if hostPrefix != "" && !strings.HasPrefix(cleaned, hostPrefix+string(filepath.Separator)) && cleaned != hostPrefix {
		cleaned = filepath.Join(hostPrefix, cleaned)
	}

	return !pathWithinRoots(cleaned, roots)
}

// pathWithinRoots reports whether target is, or is nested under, one of
// roots (checked against both the raw configured path and its canonical
// alias). Uses filepath.Rel the same way allowedpaths.Sandbox's own
// isWithinRoot does, so a root of "/" matches correctly (a naive
// rc+separator prefix check turns "/" into "//", which nothing but the
// literal path "/" would ever match).
func pathWithinRoots(target string, roots []gateRoot) bool {
	if target == "" {
		return false
	}
	clean := filepath.Clean(target)
	for _, r := range roots {
		if isWithinRoot(r.raw, clean) {
			return true
		}
		if r.canonical != "" && r.canonical != r.raw && isWithinRoot(r.canonical, clean) {
			return true
		}
	}
	return false
}

// isWithinRoot reports whether path is root itself or lexically nested
// under it. filepath.Clean does not resolve symlinks or check existence,
// which is fine here — this is a display-time gate on an already-trusted
// proc-reported string, not the filesystem access sandbox itself (that
// boundary check lives in allowedpaths.Sandbox).
func isWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// printRows writes the header and every data row, column-aligned to the
// widest cell in each column (mirroring free.go/df.go's approach). NAME is
// the last column and is never padded, so no scenario emits trailing
// whitespace.
func printRows(callCtx *builtins.CallContext, rows []row) {
	headers := []string{"COMMAND", "PID", "USER", "FD", "TYPE", "DEVICE", "SIZE/OFF", "NODE", "NAME"}
	rightAlign := []bool{false, true, false, false, false, false, true, true, false}

	all := make([][]string, 0, len(rows)+1)
	all = append(all, headers)
	for _, r := range rows {
		all = append(all, r.cells())
	}

	widths := make([]int, len(headers))
	for _, cells := range all {
		for i, c := range cells {
			if len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}

	for _, cells := range all {
		var out []byte
		for i, c := range cells {
			if i > 0 {
				out = append(out, ' ')
			}
			last := i == len(cells)-1
			switch {
			case last:
				out = append(out, c...)
			case rightAlign[i]:
				out = append(out, fmtRight(c, widths[i])...)
			default:
				out = append(out, fmtLeft(c, widths[i])...)
			}
		}
		out = append(out, '\n')
		callCtx.Out(string(out))
	}
}

func fmtLeft(s string, width int) []byte {
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
