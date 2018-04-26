// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package tailer

import (
	"io"
	"os"
	"path/filepath"
	"syscall"

	log "github.com/cihub/seelog"

	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.path)
	if err != nil {
		return err
	}
	t.tags = []string{fmt.Sprintf("filename:%s", filepath.Base(t.path))}
	log.Info("Opening ", t.path)
	f, err := os.Open(fullpath)
	if err != nil {
		return err
	}

	t.file = f
	ret, _ := f.Seek(offset, whence)
	t.readOffset = ret
	t.decodedOffset = ret

	return nil
}

// readForever lets the tailer tail the content of a file
// until it is closed or the tailer is stopped.
func (t *Tailer) readForever() {
	defer t.onStop()
	for {
		select {
		case <-t.stop:
			// stop reading data from file
			return
		default:
			// keep reading data from file
			inBuf := make([]byte, 4096)
			n, err := t.file.Read(inBuf)
			if err != nil && err != io.EOF {
				// an unexpected error occurred, stop the tailor
				t.source.Status.Error(err)
				log.Error("Err: ", err)
				return
			}
			if n == 0 {
				// wait for new data to come
				t.wait()
				continue
			}
			t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
			t.incrementReadOffset(n)
		}
	}
}

func (t *Tailer) checkForRotation() (bool, error) {
	f, err := os.Open(t.path)
	if err != nil {
		t.source.Status.Error(err)
		return false, err
	}

	stat1, err := f.Stat()
	if err != nil {
		t.source.Status.Error(err)
		return false, err
	}

	stat2, err := t.file.Stat()
	if err != nil {
		return true, nil
	}

	return inode(stat1) != inode(stat2) || stat1.Size() < t.GetReadOffset(), nil
}

// inode uniquely identifies a file on a filesystem
func inode(f os.FileInfo) uint64 {
	s := f.Sys()
	if s == nil {
		return 0
	}
	switch s := s.(type) {
	case *syscall.Stat_t:
		return uint64(s.Ino)
	default:
		return 0
	}
}
