// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths/internal/writeopen"
)

// ErrMultiplyLinkedWriteTarget reports a write target that is a hard link,
// i.e. a regular file whose inode carries more than one directory entry.
//
// AllowedPaths containment is path-based: every target is resolved through
// os.Root and a no-follow openat walk, which is sound for symlinks but blind
// to hard links, because a hard link is not a reference to a path — it is a
// second name for the same inode. A pre-existing hard link inside a
// read-write root that points at an inode also named outside every
// configured root would let a write mutate out-of-sandbox content while
// every path check still passes. Since rshell cannot enumerate an inode's
// other names, mutating write targets fail closed when the link count is
// greater than one.
var ErrMultiplyLinkedWriteTarget = errors.New("hard links are not supported as write targets")

func openWriteRoot(root *os.Root) (*os.File, error) {
	return writeopen.OpenRoot(root)
}

func closeWriteRoot(file *os.File) {
	writeopen.CloseRoot(file)
}

// openWriteFile opens relPath for writing through the no-follow openat walk,
// then rejects the resulting descriptor if it names a multiply linked regular
// file (see checkWriteTargetLinks). Every mutating primitive that
// touches file *content* — Sandbox.Open with a write flag (which backs `>`,
// `>>` and `&>` redirections), Sandbox.Truncate (`truncate`), and
// Sandbox.TruncateToZeroIfAtLeast (`logrotate`) — funnels through here, so the
// guard lives at this single choke point rather than being repeated per
// caller. Sandbox.Remove deliberately does not go through it; see the hard
// link entry in AGENTS.md for why unlinking one of several names is not
// treated as an escape.
//
// O_TRUNC is deliberately withheld from the open syscall and replayed as an
// explicit ftruncate afterwards. open(2) performs the truncation as part of
// the open itself, so an O_TRUNC open (what `>` issues) would already have
// destroyed the shared content by the time a post-open fstat could observe
// the link count — the guard has to run between the two. Splitting them is
// safe because the ftruncate targets the descriptor the open returned, not a
// re-resolved path, so nothing can be swapped in between. For non-regular
// targets the replay is skipped, matching open(2), which ignores O_TRUNC on
// FIFOs and leaves it unspecified for devices.
func (r *root) openWriteFile(relPath string, flag int, perm os.FileMode) (*os.File, error) {
	if err := r.rejectSymlinkWriteTarget(relPath); err != nil {
		return nil, err
	}
	truncateAfterOpen := flag&os.O_TRUNC != 0
	f, err := writeopen.OpenFile(r.writeRoot, r.root, relPath, flag&^os.O_TRUNC, perm)
	if err != nil {
		return nil, err
	}
	regular, err := checkWriteTargetLinks(f, relPath)
	if err != nil {
		f.Close()
		return nil, err
	}
	if truncateAfterOpen && regular {
		if err := f.Truncate(0); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// checkWriteTargetLinks fstats the already-open descriptor and rejects it when
// it is a regular file with a link count above one. It reports whether the
// descriptor names a regular file so the caller can decide whether a withheld
// O_TRUNC still needs replaying.
//
// The check runs on the fd rather than the path so it cannot be raced: the
// descriptor whose link count is inspected is exactly the descriptor the
// caller is about to mutate. This mirrors the open-then-fstat sequence
// Sandbox.Truncate already uses to reject non-regular targets, and mirrors
// the nlink != 1 skip that internal/systemd's journal vacuum has always
// applied to deletion candidates.
//
// Only regular files are considered. Directories always have a link count of
// at least two (".", plus each subdirectory's ".."), and are never valid
// write-open targets anyway; devices, FIFOs and sockets have no content that
// a second name could alias in the sense this guard protects. Platforms that
// cannot report a link count (Windows, see fileLinkCount) are not gated.
func checkWriteTargetLinks(f *os.File, relPath string) (regular bool, err error) {
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	links, ok := fileLinkCount(info)
	if !ok || links <= 1 {
		return true, nil
	}
	return true, &os.PathError{Op: "open", Path: relPath, Err: ErrMultiplyLinkedWriteTarget}
}

// removeFile unlinks relPath through an atomic, no-follow openat walk (see
// writeopen.Unlink) rather than a separate Lstat precheck followed by a
// path-based os.Root.Remove call, closing the TOCTOU window where an
// intermediate directory component could be swapped for a symlink between
// the check and the removal. The rejectSymlinkPathPrefix precheck is kept as
// defense in depth on top of the atomic walk, mirroring openWriteFile above.
func (r *root) removeFile(relPath string) error {
	if err := r.rejectSymlinkPathPrefix(relPath); err != nil {
		return err
	}
	return writeopen.Unlink(r.writeRoot, r.root, relPath)
}

func (r *root) rejectSymlinkWriteTarget(relPath string) error {
	return r.rejectSymlinkPathComponents(relPath, true)
}

// rejectSymlinkPathPrefix rejects paths whose intermediate directory
// components are symlinks, but allows the final component itself to be a
// symlink. Used by Remove: unlink(2) semantics delete the symlink named by
// the final component without following it, so a symlink is a legitimate rm
// target — only a symlinked *directory* earlier in the path is a sandbox
// escape risk.
func (r *root) rejectSymlinkPathPrefix(relPath string) error {
	return r.rejectSymlinkPathComponents(relPath, false)
}

// rejectSymlinkPathComponents walks relPath component by component and
// rejects the first symlink found. When includeLast is true (write-open
// targets), the final component is checked too, since writing through a
// symlink would redirect the write outside the resolved root. When false
// (remove targets), the final component is skipped since removing a
// symlink is expected to remove the link itself, not its referent.
func (r *root) rejectSymlinkPathComponents(relPath string, includeLast bool) error {
	clean := filepath.Clean(relPath)
	components := strings.Split(clean, string(filepath.Separator))
	var partial string
	for i, component := range components {
		if component == "" || component == "." {
			continue
		}
		if partial == "" {
			partial = component
		} else {
			partial = filepath.Join(partial, component)
		}
		if !includeLast && i == len(components)-1 {
			break
		}
		info, err := r.root.Lstat(partial)
		if err != nil {
			return nil
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return &os.PathError{Op: "open", Path: relPath, Err: writeopen.ErrSymlinkWriteTarget}
		}
	}
	return nil
}
