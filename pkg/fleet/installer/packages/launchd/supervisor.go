// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package launchd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Deadline is a wall-clock bound on an unsupervised experiment, persisted to disk so it survives
// a restart of whichever process enforces it. It is the only coverage for an experiment that is
// alive but hung: the ExitObserver sees exit, nothing else.
//
// The root watcher and the resume hook act on what this file says, so it is only trusted when it
// and the directory holding it are owned by OwnerUID and writable by nobody else: a file another
// account could have planted or replaced is refused rather than read.
type Deadline struct {
	// Path is the file the deadline is persisted to.
	Path string
	// OwnerUID is the account the file and its directory must belong to. The zero value is root,
	// which is what production uses; tests running unprivileged set their own uid.
	OwnerUID int
}

// maxDeadlineSize bounds how much of the deadline file is read. A deadline is one RFC 3339
// timestamp; anything larger was not written by Write.
const maxDeadlineSize = 256

// checkOwnership verifies that info, the result of an Lstat or an Fstat, belongs to the deadline's
// owner and is writable by no one else.
func (d Deadline) checkOwnership(path string, info os.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("could not read the ownership of %s", path)
	}
	if int(st.Uid) != d.OwnerUID {
		return fmt.Errorf("%s is owned by uid %d, expected uid %d", path, st.Uid, d.OwnerUID)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by its group or by others (mode %s)", path, info.Mode().Perm())
	}
	return nil
}

// checkDir verifies that the directory holding the deadline file is a real directory, owned by
// the deadline's owner and writable by no one else, so nobody else can plant, swap or remove the
// file in it.
func (d Deadline) checkDir() error {
	dir := filepath.Dir(d.Path)
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("could not stat the deadline directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("the deadline directory %s is not a directory", dir)
	}
	if err := d.checkOwnership(dir, info); err != nil {
		return fmt.Errorf("refusing to trust the deadline directory: %w", err)
	}
	return nil
}

// readRaw returns the deadline file's content, or "" when it is absent. The file is opened without
// following a symbolic link and must be a regular file belonging to the deadline's owner.
func (d Deadline) readRaw() (string, error) {
	if err := d.checkDir(); err != nil {
		return "", err
	}
	f, err := os.OpenFile(d.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("could not open deadline file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("could not stat deadline file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("refusing to trust the deadline file %s: not a regular file", d.Path)
	}
	if err := d.checkOwnership(d.Path, info); err != nil {
		return "", fmt.Errorf("refusing to trust the deadline file: %w", err)
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxDeadlineSize+1))
	if err != nil {
		return "", fmt.Errorf("could not read deadline file: %w", err)
	}
	if len(raw) > maxDeadlineSize {
		return "", fmt.Errorf("refusing to trust the deadline file %s: larger than %d bytes", d.Path, maxDeadlineSize)
	}
	return strings.TrimSpace(string(raw)), nil
}

// Write persists a deadline of now+window, to the nanosecond so that no two writes persist the
// same value (see Read). The write goes to a temporary file in the same directory and is renamed
// into place, matching the convention Job.Write uses for job definitions, so a reader never
// observes a partially written deadline.
func (d Deadline) Write(window time.Duration) error {
	return d.write(time.Now().Add(window).Format(time.RFC3339Nano))
}

// DeadlineReverting is what MarkReverting persists in place of a deadline. It is never a value
// Write persists, and Expired reports it as expired.
const DeadlineReverting = "reverting"

// MarkReverting replaces the deadline with DeadlineReverting: the experiment is being handed back
// to the stable job set, and is to be reverted again by whoever finds the marker still there,
// because the hand-back did not complete.
//
// Every process that read the deadline earlier sees it change, so a watcher stands down exactly as
// it does when the file is cleared, while the file itself survives a failed or interrupted
// hand-back for the next daemon start to finish.
func (d Deadline) MarkReverting() error {
	return d.write(DeadlineReverting)
}

func (d Deadline) write(deadline string) error {
	path := d.Path
	if err := d.checkDir(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("could not create temporary deadline file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(deadline); err != nil {
		tmp.Close()
		return fmt.Errorf("could not write deadline file: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("could not set deadline file mode: %w", err)
	}
	if err := tmp.Chown(0, 0); err != nil {
		log.Warnf("could not set root:wheel ownership on deadline file %s: %v", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("could not close deadline file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("could not move deadline file into place: %w", err)
	}
	return nil
}

// Clear removes the deadline file. It succeeds when the file is already absent, matching the
// idempotency convention the rest of this package follows for teardown.
func (d Deadline) Clear() error {
	if err := os.Remove(d.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("could not remove deadline file: %w", err)
	}
	return nil
}

// Present reports whether the deadline file still exists.
//
// A caller about to act on an exit event or a tick calls this immediately before acting: a
// deliberate stop clears the file first, and racing that clear is how the watcher tells a
// commanded stop apart from a real failure. There is no other signal between the two processes.
func (d Deadline) Present() (bool, error) {
	raw, err := d.readRaw()
	if err != nil {
		return false, err
	}
	return raw != "", nil
}

// Read returns the deadline exactly as persisted, or "" when the file is absent. No two Writes
// persist the same value, so it also identifies the experiment the deadline bounds: a process that
// read it earlier can tell its own experiment apart from one started since.
func (d Deadline) Read() (string, error) {
	return d.readRaw()
}

// Expired reports whether the deadline has passed, is unparseable, or is implausibly far in the
// future by more than maxSkew -- the host's clock having moved backwards since the deadline was
// written. All three are treated as expired: a hung experiment must not survive a clock that has
// become unreliable. A missing file reports not expired, since there is nothing to enforce.
func (d Deadline) Expired(maxSkew time.Duration) (bool, error) {
	raw, err := d.readRaw()
	if err != nil {
		return false, err
	}
	if raw == "" {
		return false, nil
	}
	deadline, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true, nil
	}
	now := time.Now()
	if now.After(deadline) {
		return true, nil
	}
	if deadline.Sub(now) > maxSkew {
		return true, nil
	}
	return false, nil
}

// ExitEvent is delivered when an observed process exits.
type ExitEvent struct {
	// Pid is the process that exited.
	Pid int
	// Status is the wait(2) status word delivered with the exit, or 0 for a pid that had
	// already exited by the time it was armed (see ArmExitObserver).
	Status int
}

// ExitObserver watches a fixed set of pids for exit via kqueue/EVFILT_PROC. It is one-shot per
// pid and is never re-armed: nothing relaunches an unsupervised job, so a second exit of the
// same pid cannot occur, and re-arming would only be useful for a job this package's whole
// design makes terminal by construction.
type ExitObserver struct {
	kq int
}

// ArmExitObserver opens a kqueue and arms an EVFILT_PROC/NOTE_EXIT|NOTE_EXITSTATUS watch on every
// given pid. The returned channel receives one ExitEvent per pid, including pids that had
// already exited before they could be armed: ESRCH on arming is that pid's exit notification,
// not an error to surface.
//
// Residual, unclosable risk: pid recycling between the caller reading the pid (e.g. via
// JobSet.Pids) and this function arming the filter. There is no way to close this window from
// user space; it is accepted, not fixed.
func ArmExitObserver(pids []int) (*ExitObserver, <-chan ExitEvent, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, nil, fmt.Errorf("could not open kqueue: %w", err)
	}

	events := make(chan ExitEvent, len(pids)+1)
	var immediate []int
	for _, pid := range pids {
		var kev unix.Kevent_t
		unix.SetKevent(&kev, pid, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ENABLE|unix.EV_ONESHOT)
		kev.Fflags = unix.NOTE_EXIT | unix.NOTE_EXITSTATUS
		if _, err := unix.Kevent(kq, []unix.Kevent_t{kev}, nil, nil); err != nil {
			if errors.Is(err, unix.ESRCH) {
				immediate = append(immediate, pid)
				continue
			}
			unix.Close(kq)
			return nil, nil, fmt.Errorf("could not arm exit observer for pid %d: %w", pid, err)
		}
	}
	for _, pid := range immediate {
		events <- ExitEvent{Pid: pid}
	}

	o := &ExitObserver{kq: kq}
	// The goroutine gets its own copy of the descriptor: Disarm resets the field from another
	// goroutine.
	go watchExits(kq, events, len(pids)-len(immediate))
	return o, events, nil
}

// watchExits blocks in kevent(2) until every armed, non-immediate pid has delivered its one-shot
// event, then closes the channel. remaining is 0 when every pid was already dead at arm time.
func watchExits(kq int, out chan ExitEvent, remaining int) {
	defer close(out)
	if remaining <= 0 {
		return
	}
	buf := make([]unix.Kevent_t, remaining)
	for remaining > 0 {
		n, err := unix.Kevent(kq, nil, buf, nil)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			// EBADF: the kqueue was closed from under us via Disarm. Not an error worth
			// surfacing -- the caller tore this down on purpose.
			if !errors.Is(err, unix.EBADF) {
				log.Errorf("exit observer: kevent wait failed: %v", err)
			}
			return
		}
		for i := 0; i < n; i++ {
			out <- ExitEvent{Pid: int(buf[i].Ident), Status: int(buf[i].Data)}
		}
		remaining -= n
	}
}

// Disarm closes the kqueue, ending the watch. Idempotent.
func (o *ExitObserver) Disarm() error {
	if o == nil || o.kq < 0 {
		return nil
	}
	err := unix.Close(o.kq)
	o.kq = -1
	if err != nil {
		return fmt.Errorf("could not close kqueue: %w", err)
	}
	return nil
}
