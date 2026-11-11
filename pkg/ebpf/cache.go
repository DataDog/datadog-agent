// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// reclaimedCacheDirs tracks paths whose repair warning has been logged.
var reclaimedCacheDirs sync.Map

// logCacheDirReclaimed warns (once per reclaimed path) that a stale
// cache-directory component was moved aside and recreated. A recurring reclaim
// points at something else recreating the directory as non-root, so it is worth
// surfacing rather than doing it silently.
func logCacheDirReclaimed(p string) {
	if _, logged := reclaimedCacheDirs.LoadOrStore(p, struct{}{}); !logged {
		log.Warnf("recreated cache directory component %s: it had unsafe ownership or permissions", p)
	}
}

// dedicatedDirName marks the Agent-owned subtree used by system-probe caches.
// Reclaim is confined to this subtree: a wrong-owner component is only ever
// renamed aside and deleted once we are provably inside .../datadog-agent/...,
// so a misconfigured or attacker-supplied output_dir nested under a shared,
// non-dedicated directory (e.g. /var/tmp/shared/datadog/build) is refused rather
// than causing that shared directory to be destroyed. Keep this in sync with the
// runtime_compiler_output_dir default (pkg/config/setup/system_probe_settings.go).
const dedicatedDirName = "datadog-agent"

// SecureCacheDir ensures the given directory exists, is
// not writable by group or other, and that it and every ancestor is
// a real directory owned by root. Ancestors must not be writable by other users,
// except for root-owned sticky directories. System-probe writes cached files here
// and later re-reads them as root, so the whole path must be under root's control.
//
// It verifies the already-existing prefix of the path before creating anything,
// then creates only the missing tail one component at a time. Creation never
// follows a symlink: every existing ancestor is verified as a real (non-symlink)
// root-owned directory first, so os.Mkdir of the next component resolves only
// through verified directories and returns EEXIST rather than traversing an entry
// that appeared underneath it. This is deliberately not os.MkdirAll over the
// whole path, which would resolve - and create through - a symlinked component
// before any check ran.
//
// It rejects the directory if any ancestor is a symlink, is not a directory, is
// not owned by root, or is writable by group or other without the sticky bit
// set. The sticky-bit exception permits the default location under /var/tmp
// (root-owned, mode 1777): the sticky bit keeps other users from renaming or
// deleting the datadog-agent directory that root creates beneath it.
//
// The default parent /var/tmp is shared and writable by every user, so the
// datadog-agent directory beneath it may already exist owned by a non-root user
// (left over from a previous install, or created by another process). Rather
// than refusing in that case for the lifetime of the process - which would break
// upgraded hosts and leave cache sources unavailable - SecureCacheDir
// repairs the common case: a wrong-owner directory that is both below the sticky
// boundary and inside the agent's own datadog-agent subtree is renamed aside and
// the path recreated fresh as root. Root and an entry owner can rename that
// entry inside a sticky directory. Renaming aside
// (rather than reusing or chown-ing the existing tree) means we never write
// through whatever it previously contained.
//
// The repair is deliberately narrow (see ensureCacheDirChain): it happens only
// as root, only below a verified root-owned sticky directory, only inside the
// agent's own dedicated subtree (dedicatedDirName), and only for a real directory
// with the wrong owner/permissions. This last condition is what keeps a
// misconfigured or attacker-supplied output_dir nested under a shared directory
// (e.g. /var/tmp/shared/datadog/build) from causing that shared directory to be
// renamed aside and recursively deleted. A symlink, a non-directory, anything at
// or above the sticky boundary, and anything outside the datadog-agent subtree
// are all still refused, so unexpected or broken layouts fail closed rather than
// being silently rewritten.
func SecureCacheDir(dir string) error {
	components, err := cacheDirComponents(dir)
	if err != nil {
		return err
	}

	// A single pass verifies the existing prefix, repairs one offending
	// component if needed, and creates the missing tail. The bounded retry only
	// guards against a concurrent actor racing a component between our check and
	// our create (os.Mkdir returning EEXIST on a freshly appeared entry); it is
	// not needed in the normal single-process case.
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		retryable, err := ensureCacheDirChain(components)
		if err == nil {
			return nil
		}
		if !retryable {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("unable to secure directory %s after %d attempts: %w", dir, maxAttempts, lastErr)
}

// cacheDirComponents returns the absolute form of outputDir and every ancestor
// up to the filesystem root, ordered root-first: index 0 is the filesystem root
// ("/") and the last element is the leaf output directory.
func cacheDirComponents(dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve directory %s: %w", dir, err)
	}
	var components []string
	for p := filepath.Clean(abs); ; {
		components = append(components, p)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	// Collected leaf-first; reverse so the walk below can range root -> leaf.
	slices.Reverse(components)
	return components, nil
}

// ensureCacheDirChain walks the components root -> leaf, enforcing
// verifyDirComponent on each one that already exists, creating each one that does
// not, and reclaiming (rename-aside + delete + recreate) a repairable wrong-owner
// component. Verifying the existing prefix before creating the missing tail is
// what makes creation symlink-safe: os.Mkdir of a component only ever resolves
// through ancestors already verified as real (non-symlink) root-owned
// directories.
//
// It returns whether the failure it hit (if any) is worth retrying: a race on
// creation (os.Mkdir returning EEXIST because something appeared underneath a
// verified parent) is retryable; a policy refusal or a failed reclaim is
// terminal.
//
// A component is reclaimable only when all of the following hold, so the repair
// stays conservative and never damages unrelated directories:
//   - we are running as root (system-probe's normal state); a non-root process
//     must not start renaming directories it happens not to own,
//   - the component lies strictly below a verified root-owned sticky directory
//     (a shared, writable-by-anyone parent such as /var/tmp, whose sticky bit
//     means only root can rename the entries beneath it),
//   - the component lies inside the agent's own dedicated subtree (at or below a
//     component named dedicatedDirName). Without this, the first insecure
//     ancestor on a custom output_dir could be a shared directory unrelated to
//     the agent (e.g. /var/tmp/shared), which reclaim would then rename aside and
//     recursively delete along with its contents, and
//   - the component is itself a real directory that merely has the wrong owner or
//     permissions. A symlink or non-directory at a path component is unexpected
//     rather than a routine leftover, so it is still refused (fail-closed)
//     instead of being rewritten.
func ensureCacheDirChain(components []string) (retryable bool, err error) {
	// Walk root -> leaf so we know a component sits below a verified sticky
	// boundary, and inside the agent's dedicated subtree, before we decide
	// whether its failure is reclaimable.
	belowStickyBoundary := false
	inDedicatedSubtree := false
	for i, p := range components {
		// A component named dedicatedDirName marks the start of the agent's own
		// subtree; it and anything deeper may be repaired. Set this before the
		// verify below so the datadog-agent component itself is repairable.
		if filepath.Base(p) == dedicatedDirName {
			inDedicatedSubtree = true
		}

		info, lerr := os.Lstat(p)
		if os.IsNotExist(lerr) {
			// This component and everything below it are the missing tail. Every
			// existing ancestor was verified above as a real (non-symlink)
			// root-owned directory, so os.Mkdir resolves only through verified
			// directories and will not create through a symlink at p.
			if mErr := os.Mkdir(p, 0700); mErr != nil {
				// A racing actor may have created p first; re-walking re-checks it.
				return true, fmt.Errorf("unable to create directory component %s: %w", p, mErr)
			}
			continue
		}
		if lerr != nil {
			return false, fmt.Errorf("unable to verify directory component %s: %w", p, lerr)
		}

		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return false, fmt.Errorf("unable to read ownership of directory component %s", p)
		}
		// The last component is the leaf output directory, which is held to the
		// stricter no-group/other-write rule (see verifyDirComponent).
		if verr := verifyDirComponent(p, info.Mode(), stat.Uid, i == len(components)-1); verr != nil {
			// Reclaim only a real, wrong-owner/permission directory that is below
			// the sticky boundary, inside the agent's dedicated subtree, and only
			// as root. info.Mode().IsDir() is false for a symlink or a regular
			// file, so those remain fail-closed; requiring the dedicated subtree
			// keeps reclaim from ever deleting a shared, non-agent directory.
			healable := belowStickyBoundary && inDedicatedSubtree && os.Geteuid() == 0 && info.Mode().IsDir()
			if !healable {
				return false, verr
			}
			if rErr := reclaimDirComponent(p); rErr != nil {
				return false, fmt.Errorf("%w (attempted reclaim failed: %v)", verr, rErr)
			}
			logCacheDirReclaimed(p)
			// Recreate this component fresh; deeper components were removed with
			// the reclaimed subtree and are created as the walk continues.
			if mErr := os.Mkdir(p, 0700); mErr != nil {
				return true, fmt.Errorf("unable to recreate directory component %s after reclaim: %w", p, mErr)
			}
			continue
		}
		// This component is verified good. If it is a sticky directory it marks
		// the shared-parent boundary; anything deeper may be repaired.
		if info.Mode()&os.ModeSticky != 0 {
			belowStickyBoundary = true
		}
	}
	return false, nil
}

// reclaimDirComponent moves a stale path component aside and removes it. The
// caller must only invoke this for a component that ensureCacheDirChain
// reported as repairable (below a verified root-owned sticky directory and inside
// the agent's dedicated datadog-agent subtree), which guarantees the parent is
// root-owned so root can rename the entry regardless of who owns it. Renaming
// aside (never reusing or chown-ing the existing entry) ensures we do not follow
// a symlink or reuse whatever the entry previously contained; even if the
// component's non-root owner races us (they may rename their own entry in a
// sticky parent), os.Rename does not dereference a symlink at p and we recreate
// the path fresh afterwards, so the worst case is a transient error, never
// loading or writing through an attacker-controlled path.
func reclaimDirComponent(p string) error {
	// os.Rename does not follow a symlink at p: a symlinked component is moved as
	// the link itself. The aside name is in the same (root-owned) parent so the
	// rename stays within one directory. Use the root-namespace PID: os.Getpid
	// returns a namespaced PID inside a container, which is not unique across the
	// host and could collide with a concurrent reclaim from another namespace.
	pid, err := kernel.RootNSPID()
	if err != nil {
		pid = os.Getpid()
	}
	aside := fmt.Sprintf("%s.reclaimed-%d-%d", p, pid, time.Now().UnixNano())
	if err := os.Rename(p, aside); err != nil {
		return fmt.Errorf("unable to move aside stale component %s: %w", p, err)
	}
	// Best-effort cleanup of the moved-aside tree. os.RemoveAll uses O_NOFOLLOW
	// openat internally, so it will not traverse symlinks beneath it.
	if err := os.RemoveAll(aside); err != nil {
		log.Debugf("unable to remove reclaimed component %s: %s", aside, err)
	}
	return nil
}

// verifyDirComponent enforces the cache-directory policy on a single path
// component, described by the mode returned from Lstat and its owning uid: it
// must be a real (non-symlink) directory owned by root and not writable by other
// users. It is kept separate from the filesystem walk above so the policy can be
// unit-tested with synthetic values, without needing to build a root-owned
// directory tree.
//
// The group/other-writable-but-sticky exception applies to ancestors only. It
// exists for the shared parent the default lives under (/var/tmp is 1777), which
// the agent does not own and cannot expect to tighten. The leaf output directory
// is different: it is where cache files are written and re-read under
// predictable names, so a group/other-writable leaf would let another user
// interpose an entry at one of those names between our check and the cache
// write. The leaf must therefore have no group/other write bits regardless of the
// sticky bit; isLeaf selects that stricter rule.
func verifyDirComponent(p string, mode os.FileMode, uid uint32, isLeaf bool) error {
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to use directory: %s is a symlink", p)
	}
	if !mode.IsDir() {
		return fmt.Errorf("refusing to use directory: %s is not a directory", p)
	}
	if uid != 0 {
		return fmt.Errorf("refusing to use directory: %s is not owned by root (uid=%d)", p, uid)
	}
	if mode.Perm()&0022 != 0 && (isLeaf || mode&os.ModeSticky == 0) {
		return fmt.Errorf("refusing to use directory: %s is writable by non-root (mode=%#o)", p, mode.Perm())
	}
	return nil
}
