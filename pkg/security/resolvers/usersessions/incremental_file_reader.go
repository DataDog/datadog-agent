// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package usersessions

import (
	"bufio"
	"io"
	"os"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// incrementalFileReader is used to read a file incrementally
type incrementalFileReader struct {
	path        string
	f           *os.File
	offset      int64
	mu          sync.Mutex
	ino         uint64
	parser      func(line string, sshSessionParsed *sshSessionParsed)
	stopReading chan struct{} // make(chan struct{}, 1)
}

// Init opens the file and sets the initial offset
func (ifr *incrementalFileReader) Init(f *os.File) error {
	if ifr.f != nil {
		return nil
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		seclog.Warnf("Fail to stat log file: %v", err)
		return err
	}

	ifr.offset = st.Size()

	ifr.f = f
	ifr.ino = inodeOf(st)
	_, err = ifr.f.Seek(ifr.offset, io.SeekStart)
	if err != nil {
		ifr.close()
		ifr.f = nil
	}
	return err
}

// resolveFromLogFile read all the lines that have been added since the last call without reopening the file.
// Return new lines, the byte offsets at the end of each line, and an error.
func (ifr *incrementalFileReader) resolveFromLogFile(sshSessionParsed *sshSessionParsed) error {
	if err := ifr.reloadIfRotated(); err != nil {
		return err
	}

	st, err := ifr.f.Stat()
	if err != nil {
		return err
	}

	if st.Size() == ifr.offset {
		return nil
	}
	// If the file is truncated, we restart from the beginning
	if st.Size() < ifr.offset {
		ifr.offset = 0
		if _, err := ifr.f.Seek(0, io.SeekStart); err != nil {
			return err
		}
	} else {
		// If the file is not truncated, we seek to the offset
		if _, err := ifr.f.Seek(ifr.offset, io.SeekStart); err != nil {
			return err
		}
	}

	sc := bufio.NewScanner(ifr.f)
	for sc.Scan() {
		line := sc.Text()
		ifr.parser(line, sshSessionParsed)
	}
	newOffset, err := ifr.f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	ifr.offset = newOffset
	return err
}

// close closes the file.
// The lock of IncrementalFileReader must be held
func (ifr *incrementalFileReader) close() error {
	if ifr.f == nil {
		return nil
	}
	var err error
	if ifr.f != nil {
		err = ifr.f.Close()
		ifr.f = nil
	}
	return err
}

// inodeOf get the inode of the file.
func inodeOf(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}

// reloadIfRotated reopens the file if the inode has changed.
func (ifr *incrementalFileReader) reloadIfRotated() error {
	curSt, err := os.Stat(ifr.path)
	if err != nil {
		return err
	}
	curIno := inodeOf(curSt)
	if curIno != 0 && ifr.ino != 0 && curIno != ifr.ino {
		// The file has been rotated
		if ifr.f != nil {
			_ = ifr.close()
			ifr.f = nil
		}
		f, err := os.Open(ifr.path)
		if err != nil {
			ifr.close()
			ifr.f = nil
			return err
		}
		ifr.f = f
		ifr.ino = curIno

		// We restart from the beginning because it's a new file
		ifr.offset = 0
	}
	return nil
}
