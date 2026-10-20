// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procfd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func list(ctx context.Context, procPath string, pids []int, filter ProcessFilter) ([]OpenFile, error) {
	targets, err := resolvePIDs(ctx, procPath, pids)
	if err != nil {
		return nil, err
	}

	var out []OpenFile
	for _, pid := range targets {
		if ctx.Err() != nil {
			break
		}
		remaining := MaxTotalOpenFiles - len(out)
		if remaining <= 0 {
			break
		}
		files, err := listProcess(procPath, pid, remaining, filter)
		if err != nil {
			// The process may have exited, or be inaccessible, between
			// discovery and read; skip it rather than failing the whole
			// listing (mirrors procinfo.GetByPIDs' ENOENT handling).
			continue
		}
		out = append(out, files...)
	}
	return out, nil
}

// resolvePIDs returns the PIDs to scan: the caller-supplied list verbatim
// (order preserved, no dedup — the lsof builtin handles that), or every PID
// under procPath, sorted and bounded by MaxProcesses, when none is given.
func resolvePIDs(ctx context.Context, procPath string, pids []int) ([]int, error) {
	if len(pids) > 0 {
		info, err := os.Stat(procPath)
		if err != nil {
			return nil, fmt.Errorf("lsof: cannot read %s: %w", procPath, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("lsof: cannot read %s: not a directory", procPath)
		}
		result := make([]int, len(pids))
		copy(result, pids)
		return result, nil
	}

	entries, err := os.ReadDir(procPath)
	if err != nil {
		return nil, fmt.Errorf("lsof: cannot read %s: %w", procPath, err)
	}

	var result []int
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		if len(result) >= MaxProcesses {
			break
		}
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		result = append(result, pid)
	}
	slices.Sort(result)
	return result, nil
}

// listProcess returns every open-file-like descriptor for one PID: the
// cwd/root/exe specials followed by numeric fds sorted ascending. limit
// bounds the total number of entries returned (the caller's remaining
// share of MaxTotalOpenFiles). If filter is non-nil and rejects this
// process's (pid, comm, uid), its fd directory is never scanned.
func listProcess(procPath string, pid int, limit int, filter ProcessFilter) ([]OpenFile, error) {
	pidDir := filepath.Join(procPath, strconv.Itoa(pid))

	statData, err := os.ReadFile(filepath.Join(pidDir, "stat"))
	if err != nil {
		return nil, err
	}
	comm, err := parseComm(statData)
	if err != nil {
		return nil, err
	}
	uid := readUID(pidDir)

	if filter != nil && !filter(pid, comm, uid) {
		return nil, nil
	}

	var out []OpenFile
	specials := []struct{ fd, link string }{
		{"cwd", "cwd"},
		{"rtd", "root"},
		{"txt", "exe"},
	}
	for _, sp := range specials {
		if len(out) >= limit {
			return out, nil
		}
		if of, ok := readLinkEntry(pidDir, sp.link, pid, comm, uid, sp.fd); ok {
			out = append(out, of)
		}
	}
	if len(out) >= limit {
		return out, nil
	}

	fdDir := filepath.Join(pidDir, "fd")
	fdMax := min(MaxFDsPerProcess, limit-len(out))
	fdNames, err := readFDNames(fdDir, fdMax)
	if err != nil {
		// No access to the fd directory (permission, or the process just
		// exited): return the specials gathered so far rather than
		// failing outright.
		return out, nil
	}

	type numFD struct {
		n  int
		of OpenFile
	}
	var numeric []numFD
	for _, fdNum := range fdNames {
		n, convErr := strconv.Atoi(fdNum)
		if convErr != nil {
			continue
		}
		if of, ok := readLinkEntry(fdDir, fdNum, pid, comm, uid, fdNum); ok {
			numeric = append(numeric, numFD{n, of})
		}
	}
	slices.SortFunc(numeric, func(a, b numFD) int { return a.n - b.n })
	for _, nf := range numeric {
		out = append(out, nf.of)
	}

	return out, nil
}

// readFDNames returns up to max entry names from a /proc/<pid>/fd
// directory, reading in bounded batches via ReadDir(n) rather than
// os.ReadDir (which materializes every entry before a cap can be applied).
// This bounds the memory a single call can allocate even when the
// directory itself has a pathologically large (or spoofed) entry count.
func readFDNames(fdDir string, max int) ([]string, error) {
	if max <= 0 {
		return nil, nil
	}
	f, err := os.Open(fdDir)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	const batchSize = 256
	var names []string
	for len(names) < max {
		entries, readErr := f.ReadDir(batchSize)
		for _, e := range entries {
			names = append(names, e.Name())
			if len(names) >= max {
				break
			}
		}
		if readErr != nil || len(entries) == 0 {
			break
		}
	}
	return names, nil
}

// readLinkEntry resolves one fd-like symlink (dir/linkName) into an
// OpenFile. It stats the symlink itself (not the parsed target string) so
// that deleted-but-still-open files, whose kernel-reported target no longer
// exists on disk, still yield accurate Type/Device/Size/Node — the /proc
// magic symlink continues to resolve to the live (unlinked) inode for as
// long as the fd stays open.
func readLinkEntry(dir, linkName string, pid int, comm, uid, displayFD string) (OpenFile, bool) {
	linkPath := filepath.Join(dir, linkName)

	target, err := os.Readlink(linkPath)
	if err != nil {
		return OpenFile{}, false
	}

	// Readlink and Stat below are two separate syscalls against the same
	// /proc/<pid>/fd/N magic symlink; if the process closes this fd and a
	// new one gets the same number in between, target/deleted come from
	// the old fd while Type/Device/Size/Node come from whichever file now
	// occupies fd N. There is no atomic way to read both through /proc, so
	// this torn read is accepted as a low-severity, read-only, metadata-only
	// race rather than fixed.
	var st unix.Stat_t
	if err := unix.Stat(linkPath, &st); err != nil {
		// The fd closed between directory scan and stat (a race with the
		// live process); best-effort skip rather than emit a row with
		// unknown metadata.
		return OpenFile{}, false
	}

	// The kernel appends " (deleted)" when the dentry backing this fd has
	// been unlinked. A real (non-deleted) file can also legitimately be
	// named ending in that literal string, making the two cases
	// indistinguishable from the string alone; resolving that ambiguity
	// would require stat'ing the target path itself, which is
	// process-controlled (it reflects whatever path the process opened,
	// not a hardcoded /proc path) and therefore forbidden by the file
	// access rules that scope this package's AllowedPaths exception to
	// /proc reads only (docs/RULES.md "File Access — Safe Wrappers Only").
	// The suffix is therefore always trusted as a genuine deletion marker
	// and stripped, matching what ls/lsof/readlink report on a real Linux
	// kernel: this is a known, accepted limitation, not a bug.
	deleted := false
	if trimmed, ok := strings.CutSuffix(target, " (deleted)"); ok {
		deleted = true
		target = trimmed
	}

	of := OpenFile{
		PID:     pid,
		Command: comm,
		UID:     uid,
		FD:      displayFD,
		Name:    target,
		Deleted: deleted,
		IsPath:  isRealPath(target),
	}

	of.Type = fileType(st.Mode)
	of.Device = fmt.Sprintf("%d,%d", unix.Major(st.Dev), unix.Minor(st.Dev))
	of.Node = strconv.FormatUint(st.Ino, 10)
	if of.Type == "REG" || of.Type == "DIR" {
		// SIZE/OFF reports the file's size in bytes rather than the fd's
		// read/write offset. Real lsof can show either depending on
		// flags we don't implement (-o/-s); size is what serves the
		// deleted-open-file diagnostic use case this builtin targets —
		// how much disk space an unlinked file is still holding open.
		of.Size = strconv.FormatInt(st.Size, 10)
	}

	return of, true
}

// isRealPath reports whether a /proc fd-symlink target string names a real
// filesystem path, as opposed to a socket/pipe/anon-inode target. The kernel
// never prefixes those non-path targets with a leading "/" ("socket:[12345]",
// "pipe:[12345]", "anon_inode:[eventfd]"), so the leading slash is a reliable
// signal for them.
//
// memfd targets are the one exception: the kernel reports them as
// "/memfd:name (deleted)" — WITH a leading slash (confirmed against a real
// Linux kernel; a memfd's pseudo-dentry is rooted at "/" like any other
// mount). That string is indistinguishable from a real, deleted file
// literally named "/memfd:name", so isRealPath deliberately does not treat
// the "/memfd:" prefix as a signal to exempt it from gating: doing so would
// let a real file with that literal name bypass AllowedPaths entirely (see
// TestLsofPentestMemfdNamedRealFileCannotBypassGating). The safe
// consequence is that genuine memfds are gated like any other path and show
// as "(restricted) (deleted)" outside AllowedPaths, even though they never
// name a real filesystem location.
func isRealPath(target string) bool {
	return strings.HasPrefix(target, "/")
}

func fileType(mode uint32) string {
	switch mode & unix.S_IFMT {
	case unix.S_IFREG:
		return "REG"
	case unix.S_IFDIR:
		return "DIR"
	case unix.S_IFCHR:
		return "CHR"
	case unix.S_IFBLK:
		return "BLK"
	case unix.S_IFIFO:
		return "FIFO"
	case unix.S_IFSOCK:
		return "sock"
	case unix.S_IFLNK:
		return "LINK"
	default:
		return "unknown"
	}
}

// parseComm extracts the comm field from /proc/<pid>/stat data:
// "pid (comm) state ppid ...". The comm field may itself contain spaces or
// parentheses, so it is delimited by the first '(' and the last ')'
// (matching procinfo_linux.go's readProc).
func parseComm(statData []byte) (string, error) {
	s := strings.TrimSpace(string(statData))
	openParen := strings.Index(s, "(")
	closeParen := strings.LastIndex(s, ")")
	if openParen < 0 || closeParen < 0 || closeParen <= openParen {
		return "", fmt.Errorf("lsof: malformed stat data")
	}
	return s[openParen+1 : closeParen], nil
}

// readUID reads the real UID from pidDir/status. Returns "?" if unavailable.
func readUID(pidDir string) string {
	data, err := os.ReadFile(filepath.Join(pidDir, "status"))
	if err != nil {
		return "?"
	}
	return parseUIDFromStatus(data)
}

// parseUIDFromStatus extracts the real UID from the contents of a
// /proc/<pid>/status file. Returns "?" if no Uid line is present. Split out
// from readUID so the parser can be fuzzed directly against arbitrary status
// content without per-iteration filesystem I/O.
func parseUIDFromStatus(data []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return "?"
}
