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

// ErrUntrustedCatalog means dd-procmgrd must not be started on this install root.
var ErrUntrustedCatalog = errors.New("processes.d is not trusted")

// fileOwnerUID and hasACL are indirected so the tests can run unprivileged.
var (
	fileOwnerUID = func(fi fs.FileInfo) uint32 {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			return st.Uid
		}
		return math.MaxUint32
	}
	// hasACL uses /bin/ls -lde. A GNU ls earlier on PATH rejects -e, and -e prints one line per
	// ACL entry under the mode line. An error makes the caller treat the path as untrusted.
	hasACL = func(path string) (bool, error) {
		out, err := exec.Command("/bin/ls", "-lde", "--", path).Output()
		if err != nil {
			return false, err
		}
		return len(strings.Split(strings.TrimRight(string(out), "\n"), "\n")) > 1, nil
	}
)

// trusted reports whether fi, the Lstat of path, is a real file or directory that only root
// could have written. The second value says why not.
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

// WriteDarwinProcmgrCatalog writes the embedded macOS processes.d entries under installRoot/processes.d.
//
// An error wrapping ErrUntrustedCatalog means dd-procmgrd must not be started. Any other error
// means a managed entry could not be written and nothing untrusted was left at its name.
func WriteDarwinProcmgrCatalog(installRoot string) error {
	managed, err := embedded.DarwinProcmgrProcesses()
	if err != nil {
		// A kept processes.d cannot be vetted without the managed names.
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
		// Move the directory aside whole. Repairing it would read or write a tree that is not trusted.
		quarantine := fmt.Sprintf("%s.untrusted-%d-%s", darwinProcessesDir, time.Now().UnixNano(), randomHex())
		if err := root.Rename(darwinProcessesDir, quarantine); err != nil {
			return untrustedf("%s %s and could not be moved aside: %v", dirPath, why, err)
		}
		log.Warnf("%s %s, moved it to %s", dirPath, why, filepath.Join(installRoot, quarantine))
		create = true
	}

	if create {
		// Chmod undoes the umask. The parent passed the trust check, so the directory inherits no ACL.
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
			// A leftover can be a planted symlink or a stale entry that would start a process.
			if _, lerr := root.Lstat(dst); !errors.Is(lerr, fs.ErrNotExist) {
				return untrustedf("failed to replace %s, which is still present: %v", filepath.Join(installRoot, dst), err)
			}
			errs = append(errs, fmt.Errorf("failed to write %s: %w", filepath.Join(installRoot, dst), err))
		}
	}
	return errors.Join(errs...)
}

// vetForeignEntries fails unless every yaml or yml entry dd-procmgrd would load, other than the
// managed names about to be replaced, is a trusted regular file. Dotfiles are included.
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

// replaceEntry writes the embedded entry to a temp file and renames it over name, so a symlink
// or foreign file at that name is replaced rather than followed.
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

// RemoveDarwinProcmgrCatalog removes the entries WriteDarwinProcmgrCatalog writes and removes
// processes.d if that leaves it empty. It removes nothing when the tree is not trusted.
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
