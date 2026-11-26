// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/user"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/symlink"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The macOS side of the Unix configuration directories (config_nix.go).
//
// The configuration tree belongs to the Agent's account, _dd-agent, and to the admin group, so
// anything they run can change it while the installer, running as root, reads it. Every step here
// that reads the tree as root therefore either refuses to follow a link or checks what it read.

const (
	// cpPath is the system cp: a cp from Homebrew's coreutils, earlier in PATH, does not copy
	// access control lists and extended attributes the same way.
	cpPath = "/bin/cp"
	// migrateLegacyLayout is false: macOS never had the legacy installer's managed/ layout, and its
	// migration reads the Agent-writable stable tree as root and follows links in it.
	migrateLegacyLayout = false

	// incomingPrefix names the scratch directory an experiment is built in. It is created beside
	// the stable directory, only root can enter it, and the experiment is published from it with a
	// single rename, so the Agent never reads a half-built tree.
	incomingPrefix = ".datadog-config-incoming"
	// maxDeploymentIDSize bounds how much of a deployment ID file is read.
	maxDeploymentIDSize = 256
)

// stageExperiment builds the experiment in a scratch directory only root can reach, and publishes
// it at the experiment path with one rename. fill then never runs in a tree the Agent's account can
// change, and a failure at any point leaves the experiment path as it was.
func (d *Directories) stageExperiment(ctx context.Context, fill func(dir string) error) (err error) {
	parent := filepath.Dir(d.StablePath)
	discardStaleScratch(parent)
	scratch, err := os.MkdirTemp(parent, incomingPrefix)
	if err != nil {
		return fmt.Errorf("could not create the scratch configuration directory: %w", err)
	}
	defer discardScratch(scratch)
	dir := filepath.Join(scratch, filepath.Base(d.StablePath))
	if err := copyDirectory(ctx, d.StablePath, dir); err != nil {
		return fmt.Errorf("could not copy %s: %w", d.StablePath, err)
	}
	if err := checkCopy(d.StablePath, dir); err != nil {
		return err
	}
	// The stable deployment ID came along with the copy: only fill writes the experiment's.
	if err := os.RemoveAll(filepath.Join(dir, deploymentIDFile)); err != nil {
		return err
	}
	if err := fill(dir); err != nil {
		return err
	}
	if err := paths.RemoveAll(ctx, d.ExperimentPath); err != nil {
		return err
	}
	if err := os.Rename(dir, d.ExperimentPath); err != nil {
		if restErr := symlink.Set(d.ExperimentPath, d.StablePath); restErr != nil {
			log.Warnf("could not put %s back to rest on %s: %v", d.ExperimentPath, d.StablePath, restErr)
		}
		return fmt.Errorf("could not publish the configuration experiment: %w", err)
	}
	return nil
}

// checkCopy checks the copy cp made of the stable tree, in the scratch directory, against the
// stable tree, and makes it safe to patch and later remove.
//
// cp read the stable tree by path, as root, while the Agent's account could change it. So:
//   - a regular file must come from a source file with a single link. Otherwise it may be a hard
//     link the account made to a file it cannot read: cp would have turned it into an independent
//     copy, and a later .dmg install (secureConfig) would hand that copy to the account;
//   - a regular file the tree's owner does not own is read again from the source, through a
//     handle that does not follow a link, and must match its owner and mode, so it holds only what
//     a file of that owner at that path holds;
//   - file flags cp carried over (uchg, uappnd and the like) are cleared: a flagged file could not
//     be patched, and a flagged copy could not be removed by a stop.
func checkCopy(source, copied string) error {
	src, err := os.OpenRoot(source)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", source, err)
	}
	defer src.Close()
	rootInfo, err := os.Lstat(copied)
	if err != nil {
		return err
	}
	treeOwner, _, ok := ownership(rootInfo)
	if !ok {
		return fmt.Errorf("could not read the ownership of %s", copied)
	}
	return filepath.WalkDir(copied, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(copied, path)
		if err != nil {
			return err
		}
		if err := clearFlags(path); err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		f, err := src.OpenFile(rel, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return fmt.Errorf("could not check %s against the configuration: %w", rel, err)
		}
		defer f.Close()
		sourceInfo, err := f.Stat()
		if err != nil {
			return err
		}
		st, ok := sourceInfo.Sys().(*syscall.Stat_t)
		if !sourceInfo.Mode().IsRegular() || !ok || st.Nlink != 1 {
			return fmt.Errorf("refusing to copy %s: it is linked from elsewhere, or is no longer a regular file", rel)
		}
		if uid, _, ok := ownership(info); ok && uid != treeOwner {
			return recopy(f, sourceInfo, path, info, rel)
		}
		return nil
	})
}

// recopy replaces the copy at path with the content of the open source file, once their owner and
// mode match.
func recopy(source *os.File, sourceInfo fs.FileInfo, path string, info fs.FileInfo, rel string) error {
	sourceUID, sourceGID, _ := ownership(sourceInfo)
	uid, gid, _ := ownership(info)
	if sourceUID != uid || sourceGID != gid || sourceInfo.Mode().Perm() != info.Mode().Perm() {
		return fmt.Errorf("refusing to copy %s: it changed while it was copied", rel)
	}
	target, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("could not rewrite the copy of %s: %w", rel, err)
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return fmt.Errorf("could not rewrite the copy of %s: %w", rel, err)
	}
	return target.Close()
}

// clearFlags clears the file flags of path, without following a link.
func clearFlags(path string) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return err
	}
	if st.Flags == 0 {
		return nil
	}
	if err := unix.Chflags(path, 0); err != nil {
		return fmt.Errorf("could not clear the file flags of %s: %w", path, err)
	}
	return nil
}

// discardScratch removes a scratch directory, clearing any file flags in it first so a copy that
// failed part-way is removed too.
func discardScratch(dir string) {
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type()&fs.ModeSymlink == 0 {
			_ = clearFlags(path)
		}
		return nil
	})
	if err := os.RemoveAll(dir); err != nil {
		log.Warnf("could not discard the scratch configuration directory %s: %v", dir, err)
	}
}

// discardStaleScratch removes any leftover scratch directories from a WriteExperiment that
// crashed before cleaning up after itself. It is best effort: a failure here shouldn't stop a new
// experiment from being written.
func discardStaleScratch(parent string) {
	stale, err := filepath.Glob(filepath.Join(parent, incomingPrefix+"*"))
	if err != nil {
		log.Warnf("could not look for stale scratch configuration directories: %v", err)
		return
	}
	for _, dir := range stale {
		discardScratch(dir)
	}
}

// renameSwap exchanges two paths in one step. A var so tests can make it fail.
var renameSwap = func(from, to string) error {
	return unix.RenamexNp(from, to, unix.RENAME_SWAP)
}

// exchangeDirectories puts the deployed experiment at the stable path in one step, leaving the
// previous stable directory at the experiment path.
//
// The stable deployment ID is removed first: the previous stable directory ends up at the
// experiment path, and must not read as a deployed experiment if the promote stops there.
func exchangeDirectories(stable, experiment string) error {
	info, err := os.Lstat(experiment)
	if err != nil {
		return err
	}
	id, _ := readDeploymentID(filepath.Join(experiment, deploymentIDFile))
	if !info.IsDir() || len(id) == 0 {
		return fmt.Errorf("no configuration experiment is deployed at %s", experiment)
	}
	idPath := filepath.Join(stable, deploymentIDFile)
	previous, _ := readDeploymentID(idPath)
	if err := os.RemoveAll(idPath); err != nil {
		return err
	}
	if err := renameSwap(experiment, stable); err != nil {
		if len(previous) > 0 {
			if restoreErr := os.WriteFile(idPath, previous, 0640); restoreErr != nil {
				log.Warnf("could not restore the stable deployment ID: %v", restoreErr)
			}
		}
		if errors.Is(err, unix.ENOTSUP) {
			return errors.ErrUnsupported
		}
		return err
	}
	return nil
}

// readDeploymentID reads a deployment ID from the Agent's configuration tree without following a
// final link, blocking on a fifo or failing on a directory: anything but a small regular file of
// printable characters reads as no ID.
func readDeploymentID(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, syscall.ELOOP) {
		log.Warnf("ignoring the deployment ID at %s: it is a link", path)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxDeploymentIDSize {
		log.Warnf("ignoring the deployment ID at %s: not a small regular file", path)
		return nil, nil
	}
	id, err := io.ReadAll(io.LimitReader(f, maxDeploymentIDSize))
	if err != nil {
		return nil, err
	}
	if !isPrintable(id) {
		log.Warnf("ignoring the deployment ID at %s: it holds unprintable characters", path)
		return nil, nil
	}
	return id, nil
}

// isPrintable reports whether id holds only printable ASCII characters.
func isPrintable(id []byte) bool {
	for _, c := range id {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// ownership returns the uid and gid a file is owned by.
func ownership(info fs.FileInfo) (int, int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(stat.Uid), int(stat.Gid), true
}

// setFileOwnershipAndPermissions sets the ownership and permissions for a file, through a handle
// that refuses to resolve to anything but the file itself. If the account doesn't exist (e.g. in
// tests) or the process doesn't have permission to change ownership, the function logs a warning
// and continues without failing.
//
// The owners and modes in the configFileSpec describe the Linux layout. On macOS every
// configuration file has the same ownership and mode, the ones the installer's .dmg set-up gives
// the whole tree, so a file written by Fleet is indistinguishable from one installed by the .dmg.
func setFileOwnershipAndPermissions(ctx context.Context, root *os.Root, path string, _ *configFileSpec) error {
	f, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("error opening %s: %w", path, err)
	}
	defer f.Close()
	if err := f.Chmod(agentConfigFileMode); err != nil {
		return fmt.Errorf("error setting file permissions for %s: %w", path, err)
	}
	uid, err := user.GetUserID(ctx, AgentUser)
	if err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
		return nil
	}
	gid, err := user.GetGroupID(ctx, AgentGroup)
	if err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
		return nil
	}
	if err := f.Chown(uid, gid); err != nil {
		log.Warnf("error setting file ownership for %s: %v", path, err)
	}
	return nil
}
