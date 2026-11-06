// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package processmanager implements fleet installer helpers for dd-procmgr's processes.d catalog
// on macOS.
package processmanager

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const darwinProcessesDir = "processes.d"

// ErrUntrustedCatalog means the install root or processes.d could not be established as
// trusted, so dd-procmgrd must not be started on it.
var ErrUntrustedCatalog = errors.New("processes.d is not trusted")

// fileOwnerUID and hasACL are indirected so the tests can run unprivileged.
var (
	fileOwnerUID = func(fi fs.FileInfo) uint32 {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			return st.Uid
		}
		return math.MaxUint32
	}
	// hasACL names ls by absolute path: a GNU ls earlier on PATH rejects -e. With -e, ls prints
	// one line per ACL entry under the mode line. An error is returned to the caller, which
	// treats the path as untrusted.
	hasACL = func(path string) (bool, error) {
		out, err := exec.Command("/bin/ls", "-lde", "--", path).Output()
		if err != nil {
			return false, err
		}
		return len(strings.Split(strings.TrimRight(string(out), "\n"), "\n")) > 1, nil
	}
)

// trusted reports whether path is something only root could have written: a real directory or
// regular file, not a symlink, owned by uid 0, with no group or other write bit, and with no
// ACL. fi is the Lstat of path. When it is not, the second value says why.
func trusted(path string, fi fs.FileInfo) (bool, string) {
	mode := fi.Mode()
	if mode&fs.ModeSymlink != 0 {
		return false, "is a symlink"
	}
	if !mode.IsDir() && !mode.IsRegular() {
		return false, "is neither a directory nor a regular file"
	}
	if uid := fileOwnerUID(fi); uid != 0 {
		return false, fmt.Sprintf("is owned by uid %d", uid)
	}
	if mode.Perm()&0o022 != 0 {
		return false, fmt.Sprintf("is writable by group or others (mode %#o)", mode.Perm())
	}
	acl, err := hasACL(path)
	if err != nil {
		return false, fmt.Sprintf("could not be checked for an ACL: %v", err)
	}
	if acl {
		return false, "carries an ACL"
	}
	return true, ""
}

func untrustedf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUntrustedCatalog, fmt.Sprintf(format, args...))
}

func randomHex() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// WriteDarwinProcmgrCatalog writes the macOS processes.d entries the installer embeds under
// installRoot/processes.d.
//
// dd-procmgrd runs as root and starts what processes.d names, so nothing in it is trusted unless
// root alone could have put it there. installRoot must be trusted or nothing is written. A
// processes.d that is not trusted is moved aside, not repaired, and a new one is created: its
// contents may have been planted while it was open. Each entry is replaced, never written
// through, so an existing symlink or foreign-owned file at that name is not followed.
//
// An error wrapping ErrUntrustedCatalog means processes.d is not safe to start dd-procmgrd on.
// Any other error means some entries could not be written into a trusted processes.d.
func WriteDarwinProcmgrCatalog(installRoot string) error {
	managed, err := embedded.DarwinProcmgrProcesses()
	if err != nil {
		// Without the managed names, a kept processes.d cannot be vetted.
		return untrustedf("failed to list the embedded processes.d entries: %v", err)
	}

	rootInfo, err := os.Lstat(installRoot)
	if err != nil {
		return untrustedf("install root %s: %v", installRoot, err)
	}
	if ok, why := trusted(installRoot, rootInfo); !ok {
		return untrustedf("install root %s %s", installRoot, why)
	}
	if !rootInfo.IsDir() {
		return untrustedf("install root %s is not a directory", installRoot)
	}

	root, err := os.OpenRoot(installRoot)
	if err != nil {
		return untrustedf("failed to open install root %s: %v", installRoot, err)
	}
	defer root.Close()

	dirPath := filepath.Join(installRoot, darwinProcessesDir)
	create := false
	dirInfo, err := root.Lstat(darwinProcessesDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		create = true
	case err != nil:
		return untrustedf("%s: %v", dirPath, err)
	default:
		ok, why := trusted(dirPath, dirInfo)
		if ok && !dirInfo.IsDir() {
			ok, why = false, "is not a directory"
		}
		if ok {
			if err := vetForeignEntries(root, installRoot, managed); err != nil {
				return err
			}
			break
		}
		// Never chmod, read or write into an untrusted processes.d: move it aside whole.
		quarantine := fmt.Sprintf("%s.untrusted-%d-%s", darwinProcessesDir, time.Now().UnixNano(), randomHex())
		if err := root.Rename(darwinProcessesDir, quarantine); err != nil {
			return untrustedf("%s %s and could not be moved aside: %v", dirPath, why, err)
		}
		log.Warnf("%s %s, moved it to %s", dirPath, why, filepath.Join(installRoot, quarantine))
		create = true
	}

	if create {
		// The parent passed the trust check, so the new directory inherits no ACL. The chmod
		// undoes the umask.
		if err := root.Mkdir(darwinProcessesDir, 0o755); err != nil {
			return untrustedf("failed to create %s: %v", dirPath, err)
		}
		if err := root.Chmod(darwinProcessesDir, 0o755); err != nil {
			return untrustedf("failed to set the mode of %s: %v", dirPath, err)
		}
	}

	var errs []error
	for _, name := range managed {
		dst := path.Join(darwinProcessesDir, name)
		if err := replaceEntry(root, name); err != nil {
			// A leftover can be a planted symlink or a trusted stale entry that would start
			// something, and dd-procmgrd must not load either.
			if _, lerr := root.Lstat(dst); !errors.Is(lerr, fs.ErrNotExist) {
				return untrustedf("failed to replace %s, which is still present: %v", filepath.Join(installRoot, dst), err)
			}
			errs = append(errs, fmt.Errorf("failed to write %s: %w", filepath.Join(installRoot, dst), err))
		}
	}
	return errors.Join(errs...)
}

// vetForeignEntries fails unless every entry in a kept processes.d that dd-procmgrd would load,
// other than the managed names that are about to be replaced, is a trusted regular file.
// dd-procmgrd loads every name with a yaml or yml extension, dotfiles included. A name that is
// exactly .yaml is judged too, which is stricter than what dd-procmgrd loads.
func vetForeignEntries(root *os.Root, installRoot string, managed []string) error {
	dirPath := filepath.Join(installRoot, darwinProcessesDir)
	dir, err := root.Open(darwinProcessesDir)
	if err != nil {
		return untrustedf("failed to open %s: %v", dirPath, err)
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return untrustedf("failed to list %s: %v", dirPath, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if slices.Contains(managed, name) {
			continue
		}
		if ext := filepath.Ext(name); ext != ".yaml" && ext != ".yml" {
			continue
		}
		entryPath := filepath.Join(dirPath, name)
		fi, err := root.Lstat(path.Join(darwinProcessesDir, name))
		if err != nil {
			return untrustedf("%s: %v", entryPath, err)
		}
		ok, why := trusted(entryPath, fi)
		if ok && !fi.Mode().IsRegular() {
			ok, why = false, "is not a regular file"
		}
		if !ok {
			return untrustedf("%s %s", entryPath, why)
		}
	}
	return nil
}

// replaceEntry writes the embedded entry name to a temp file in processes.d and renames it over
// name, so whatever was at name is replaced without being followed. The temp file is removed on
// failure.
func replaceEntry(root *os.Root, name string) error {
	content, err := embedded.GetDarwinProcmgrProcess(name)
	if err != nil {
		return err
	}
	tmp := path.Join(darwinProcessesDir, "."+name+".tmp-"+randomHex())
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		_ = f.Close()
		_ = root.Remove(tmp)
		return err
	}
	if _, err := f.Write(content); err != nil {
		return fail(err)
	}
	if err := f.Chmod(0o644); err != nil {
		return fail(err)
	}
	// Synced before the rename, so dd-procmgrd never finds a zero-length entry.
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	if err := root.Rename(tmp, path.Join(darwinProcessesDir, name)); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	return nil
}

// RemoveDarwinProcmgrCatalog removes the entries WriteDarwinProcmgrCatalog writes, leaves any
// other file alone, and removes processes.d if that left it empty. It removes nothing when
// installRoot or processes.d is not trusted. Missing files are not errors.
func RemoveDarwinProcmgrCatalog(installRoot string) error {
	dirPath := filepath.Join(installRoot, darwinProcessesDir)
	for _, p := range []string{installRoot, dirPath} {
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		ok, why := trusted(p, fi)
		if ok && !fi.IsDir() {
			ok, why = false, "is not a directory"
		}
		if !ok {
			log.Warnf("not removing the processes.d catalog: %s %s", p, why)
			return nil
		}
	}

	// processes.d is opened as its own root, so no symlink above it is traversed.
	root, err := os.OpenRoot(dirPath)
	if err != nil {
		return err
	}
	managed, err := embedded.DarwinProcmgrProcesses()
	if err != nil {
		root.Close()
		return err
	}
	var errs []error
	for _, name := range managed {
		if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", filepath.Join(dirPath, name), err))
		}
	}
	empty, err := isEmptyDir(root)
	root.Close()
	if err != nil {
		errs = append(errs, err)
	} else if empty {
		if err := os.Remove(dirPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", dirPath, err))
		}
	}
	return errors.Join(errs...)
}

func isEmptyDir(root *os.Root) (bool, error) {
	dir, err := root.Open(".")
	if err != nil {
		return false, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(1)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return len(entries) == 0, nil
}
