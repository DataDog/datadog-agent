// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package packages

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// stateWatchPoll bounds each wait for a change to the deadline's directory, so the watch notices it
// was stopped. A var so tests can shorten it.
var stateWatchPoll = time.Second

// watchExperimentState watches the directory that holds the experiment deadline
// (configExperimentDeadlinePath), and delivers an event on the returned channel each time an entry
// in it is added, removed or renamed. Writing the deadline renames a temporary file into place, and
// clearing it removes it (launchd.Deadline), so every change to the deadline -- a stop or promote
// marking it as reverting, then clearing it -- reaches the supervising watcher at once, rather than
// on its next deadline tick. Events are coalesced: one pending event stands for any number of
// changes, and the watcher re-reads the deadline anyway. The returned function ends the watch.
//
// The directory is opened for event notification only (O_EVTONLY), so the watch does not keep its
// volume busy.
func watchExperimentState() (<-chan struct{}, func(), error) {
	dir := filepath.Dir(configExperimentDeadlinePath)
	fd, err := unix.Open(dir, unix.O_EVTONLY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("could not open %s to watch it: %w", dir, err)
	}
	kq, err := unix.Kqueue()
	if err != nil {
		unix.Close(fd)
		return nil, nil, fmt.Errorf("could not open kqueue: %w", err)
	}
	var kev unix.Kevent_t
	unix.SetKevent(&kev, fd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_ENABLE|unix.EV_CLEAR)
	kev.Fflags = unix.NOTE_WRITE
	if _, err := unix.Kevent(kq, []unix.Kevent_t{kev}, nil, nil); err != nil {
		unix.Close(kq)
		unix.Close(fd)
		return nil, nil, fmt.Errorf("could not watch %s: %w", dir, err)
	}

	changes := make(chan struct{}, 1)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		events := make([]unix.Kevent_t, 1)
		timeout := unix.NsecToTimespec(stateWatchPoll.Nanoseconds())
		for {
			select {
			case <-done:
				return
			default:
			}
			n, err := unix.Kevent(kq, nil, events, &timeout)
			if err != nil {
				if errors.Is(err, unix.EINTR) {
					continue
				}
				return
			}
			if n == 0 {
				continue
			}
			select {
			case changes <- struct{}{}:
			default:
			}
		}
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(done)
			<-finished
			unix.Close(kq)
			unix.Close(fd)
		})
	}
	return changes, stop, nil
}
