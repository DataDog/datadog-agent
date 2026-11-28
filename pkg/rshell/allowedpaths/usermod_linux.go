// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package allowedpaths

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// POSIX record locks are process-wide, so also serialize runners in this process.
var usermodMu sync.Mutex

// AddDDAgentToDocker performs the restricted in-place account-file POC.
// Scripts cannot select its directory, user, group, or replacement contents.
func (s *Sandbox) AddDDAgentToDocker(ctx context.Context) error {
	return s.addDDAgentToDocker(ctx, "/etc")
}

func (s *Sandbox) addDDAgentToDocker(ctx context.Context, dir string) (err error) {
	if s == nil || s.readOnly {
		return errors.New("writable /etc in AllowedPaths is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !usermodMu.TryLock() {
		return errors.New("account database is busy")
	}
	defer usermodMu.Unlock()
	ar, rel, ok := s.resolveWriteTarget(dir)
	if !ok {
		return errors.New("writable /etc in AllowedPaths is required")
	}
	if ar.absPath != ar.canonicalAbsPath {
		return errors.New("symlinked account roots are not supported")
	}
	if err := ar.rejectSymlinkWriteTarget(rel); err != nil {
		return err
	}
	info, err := ar.root.Lstat(rel)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o022 != 0 || !accountOwnedByCaller(info) {
		return errors.New("account directory must be owned by the effective user and not writable by others")
	}
	lockPath := filepath.Join(dir, ".pwd.lock")
	lock, err := s.openAccountFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open account lock: %w", err)
	}
	defer func() {
		if closeErr := lock.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	flock := unix.Flock_t{Type: unix.F_WRLCK, Whence: int16(io.SeekStart)}
	if err := unix.FcntlFlock(lock.Fd(), unix.F_SETLK, &flock); err != nil {
		return fmt.Errorf("account database is busy or cannot be locked: %w", err)
	}
	// shortcut: only lckpwdf-compatible editors serialize here; require idle account tools until PID-lock publication is sandbox-supported.
	for _, name := range []string{"passwd.lock", "group.lock", "gshadow.lock"} {
		if _, err := ar.root.Lstat(filepath.Join(rel, name)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("account editor lock exists or cannot be inspected")
		}
	}
	var snapshots []*accountSnapshot
	defer func() {
		for _, file := range snapshots {
			if closeErr := file.file.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close %s (updates may have completed): %w", file.path, closeErr))
			}
		}
	}()
	for _, name := range []string{"passwd", "gshadow", "group"} {
		flags := os.O_RDWR
		if name == "passwd" {
			flags = os.O_RDONLY
		}
		path := filepath.Join(dir, name)
		file, openErr := s.openAccountFile(path, flags, 0)
		if name == "gshadow" && errors.Is(openErr, os.ErrNotExist) {
			continue
		}
		if openErr != nil {
			return fmt.Errorf("open %s: %w", name, openErr)
		}
		snapshot := &accountSnapshot{file: file, path: path}
		snapshots = append(snapshots, snapshot)
		snapshot.info, err = file.Stat()
		if err != nil {
			return err
		}
		snapshot.before, err = readAccountFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if name == "passwd" {
			err = validateDDAgent(snapshot.before)
		} else {
			snapshot.after, err = appendDockerMember(snapshot.before, name == "gshadow")
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	// Validate every file before creating recovery copies or writing any database.
	for _, file := range snapshots {
		if err := s.checkAccountSnapshot(file); err != nil {
			return err
		}
	}
	backupID := rand.Text()
	for _, file := range snapshots {
		if file.after == nil || bytes.Equal(file.before, file.after) {
			continue
		}
		file.backup = file.path + ".rshell-usermod-" + backupID
		backup, err := s.openAccountFile(file.backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("create recovery copy: %w", err)
		}
		_, writeErr := writeChunkedCancellable(ctx, backup, file.before)
		syncErr := backup.Sync()
		closeErr := backup.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return fmt.Errorf("recovery copy %s incomplete; databases unchanged: %w", file.backup, err)
		}
	}
	// Persist recovery-copy directory entries before modifying either database.
	directory, err := ar.root.Open(rel)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return fmt.Errorf("sync recovery directory; databases unchanged: %w", err)
	}
	for _, file := range snapshots {
		if err := s.checkAccountSnapshot(file); err != nil {
			return err
		}
	}
	changed := false
	for _, file := range snapshots {
		if file.backup == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("stopped (earlier changes=%t, recovery copy %s): %w", changed, file.backup, err)
		}
		if err := s.checkAccountSnapshot(file); err != nil {
			return fmt.Errorf("stopped (earlier changes=%t, recovery copy %s): %w", changed, file.backup, err)
		}
		if _, err := file.file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("stopped (earlier changes=%t, recovery copy %s): %w", changed, file.backup, err)
		}
		// shortcut: in-place growth is not crash-atomic; keep recovery copies until a transactional backend is reviewed.
		_, writeErr := writeChunkedCancellable(ctx, file.file, file.after)
		syncErr := file.file.Sync()
		if err := errors.Join(writeErr, syncErr); err != nil {
			return fmt.Errorf("%s may be partially updated (earlier changes=%t); recovery copy %s: %w", file.path, changed, file.backup, err)
		}
		changed = true
		written := *file
		written.before = file.after
		if err := s.checkAccountSnapshot(&written); err != nil {
			return fmt.Errorf("%s was updated but verification failed; recovery copy %s: %w", file.path, file.backup, err)
		}
	}
	return nil
}

type accountSnapshot struct {
	file          *os.File
	path          string
	info          fs.FileInfo
	before, after []byte
	backup        string
}

func (s *Sandbox) openAccountFile(path string, flags int, mode os.FileMode) (*os.File, error) {
	ar, rel, ok := s.resolveWriteTarget(path)
	if !ok {
		return nil, os.ErrPermission
	}
	file, err := ar.openWriteFile(rel, flags|unix.O_NONBLOCK, mode)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && (!info.Mode().IsRegular() || info.Mode()&^os.ModePerm != 0 || info.Mode().Perm()&0o022 != 0 || !accountOwnedByCaller(info)) {
		err = errors.New("account target must be a regular file owned by the effective user and not writable by others")
	}
	if err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func readAccountFile(file *os.File) ([]byte, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAccountFileBytes+1))
	if err == nil && len(data) > maxAccountFileBytes {
		err = errors.New("account file exceeds size limit")
	}
	return data, err
}

func (s *Sandbox) checkAccountSnapshot(file *accountSnapshot) error {
	ar, rel, ok := s.resolveWriteTarget(file.path)
	if !ok {
		return os.ErrPermission
	}
	info, err := ar.root.Lstat(rel)
	if err != nil {
		return err
	}
	original, originalOK := file.info.Sys().(*syscall.Stat_t)
	current, currentOK := info.Sys().(*syscall.Stat_t)
	if !os.SameFile(file.info, info) || info.Mode() != file.info.Mode() || !accountOwnedByCaller(info) ||
		!originalOK || !currentOK || original.Gid != current.Gid {
		return errors.New("account file identity or metadata changed")
	}
	if _, err := checkWriteTargetLinks(file.file, rel); err != nil {
		return err
	}
	data, err := readAccountFile(file.file)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, file.before) {
		return errors.New("account file contents changed")
	}
	return nil
}

func accountOwnedByCaller(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}
