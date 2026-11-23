// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package procmgrdecision

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// owner is the uid and gid that must own the record directory and file.
type owner struct {
	uid int
	gid int
}

// DefaultStore is the production store: /opt/datadog-packages/process_manager,
// owned by root.
func DefaultStore() *Store {
	return newStore(paths.PackagesPath, 0, 0)
}

// newStore returns a store whose record is dir/process_manager. The directory and
// the file must be owned by uid and gid. Tests pass the current user, so no test
// needs root.
func newStore(dir string, uid, gid int) *Store {
	return &Store{path: filepath.Join(dir, recordName), owner: owner{uid: uid, gid: gid}}
}

// openRecord runs the unix checks of section 5.2 and opens the record. found is
// false when the directory or the file does not exist. The returned file has
// passed the file trust check on its descriptor.
func (s *Store) openRecord() (*os.File, bool, error) {
	if _, err := os.Lstat(s.dir()); errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("checking %s: %w", s.dir(), err)
	}
	d, err := openDirNoFollow(s.dir())
	if err != nil {
		return nil, false, err
	}
	err = s.checkDir(d)
	_ = d.Close()
	if err != nil {
		return nil, false, err
	}

	// O_NONBLOCK so that a FIFO at the path cannot block the open. checkFile rejects it.
	f, err := os.OpenFile(s.path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	case errors.Is(err, syscall.ELOOP):
		return nil, false, fmt.Errorf("%w: %s is a symbolic link", ErrUntrusted, s.path)
	case err != nil:
		return nil, false, fmt.Errorf("opening %s: %w", s.path, err)
	}
	if err := s.checkFile(f); err != nil {
		_ = f.Close()
		return nil, false, err
	}
	return f, true, nil
}

// prepareDir creates or repairs the record directory and verifies it. It returns
// the open directory so that the caller can fsync it after the rename. The caller
// closes it.
func (s *Store) prepareDir() (*os.File, error) {
	created := false
	if _, err := os.Lstat(s.dir()); errors.Is(err, fs.ErrNotExist) {
		// Only the directory itself is created: its parent must already exist.
		switch err := os.Mkdir(s.dir(), 0o755); {
		case err == nil:
			created = true
		case errors.Is(err, fs.ErrExist):
			// Another process created it first. Continue as for an existing directory.
		default:
			return nil, fmt.Errorf("creating %s: %w", s.dir(), err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("checking %s: %w", s.dir(), err)
	}
	d, err := openDirNoFollow(s.dir())
	if err != nil {
		return nil, err
	}
	if err := s.repairDir(d, created); err != nil {
		_ = d.Close()
		return nil, err
	}
	if err := s.checkDir(d); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

// openDirNoFollow opens a directory without following a symlink at its path. A
// symlink, or a path that is not a directory, is ErrUntrusted and is never repaired.
func openDirNoFollow(dir string) (*os.File, error) {
	d, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	switch {
	case errors.Is(err, syscall.ELOOP), errors.Is(err, syscall.ENOTDIR):
		return nil, fmt.Errorf("%w: %s is a symbolic link or not a directory", ErrUntrusted, dir)
	case err != nil:
		return nil, fmt.Errorf("opening %s: %w", dir, err)
	}
	return d, nil
}

// repairDir sets the owner and the mode of an open directory. It works through
// the descriptor, never by path, so the path cannot be swapped under the change.
func (s *Store) repairDir(d *os.File, created bool) error {
	fi, err := d.Stat()
	if err != nil {
		return fmt.Errorf("checking %s: %w", s.dir(), err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: cannot read owner of %s", ErrUntrusted, s.dir())
	}
	if int(st.Uid) != s.uid || int(st.Gid) != s.gid {
		if err := d.Chown(s.uid, s.gid); err != nil {
			return fmt.Errorf("%w: cannot set owner of %s: %v", ErrUntrusted, s.dir(), err)
		}
	}
	perm := fi.Mode().Perm()
	switch {
	case created:
		// The umask may have removed bits from the mode passed to Mkdir.
		if err := d.Chmod(0o755); err != nil {
			return fmt.Errorf("%w: cannot set mode of %s: %v", ErrUntrusted, s.dir(), err)
		}
	case perm&0o022 != 0:
		if err := d.Chmod(perm &^ 0o022); err != nil {
			return fmt.Errorf("%w: cannot set mode of %s: %v", ErrUntrusted, s.dir(), err)
		}
	}
	return nil
}

// checkDir is the directory trust check on an open directory: owned by the
// expected uid, and not group or world writable.
func (s *Store) checkDir(d *os.File) error {
	fi, err := d.Stat()
	if err != nil {
		return fmt.Errorf("checking %s: %w", s.dir(), err)
	}
	return s.trusted(fi, s.dir())
}

// checkFile is the file trust check on an open record: a regular file owned by the
// expected uid, and not group or world writable.
func (s *Store) checkFile(f *os.File) error {
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("checking %s: %w", s.path, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrUntrusted, s.path)
	}
	return s.trusted(fi, s.path)
}

// trusted checks the owner uid and the write bits of a stat result that was taken
// from an open descriptor.
func (s *Store) trusted(fi fs.FileInfo, name string) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: cannot read owner of %s", ErrUntrusted, name)
	}
	if int(st.Uid) != s.uid {
		return fmt.Errorf("%w: %s is owned by uid %d, expected %d", ErrUntrusted, name, st.Uid, s.uid)
	}
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return fmt.Errorf("%w: %s has mode %o, group or world writable", ErrUntrusted, name, perm)
	}
	return nil
}

// chmodRecord sets the mode of the temporary record before the rename. os.CreateTemp
// creates it 0600.
func chmodRecord(f *os.File) error {
	return f.Chmod(0o644)
}

// syncDir makes the rename durable.
func syncDir(d *os.File) error {
	return d.Sync()
}
