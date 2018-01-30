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

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
)

func (t *Tailer) startReading(offset int64, whence int) error {
	fullpath, err := filepath.Abs(t.path)
	if err != nil {
		t.source.Status.Error(err)
		return err
	}
	log.Info("Opening ", t.path)
	f, err := os.Open(fullpath)
	if err != nil {
		t.source.Status.Error(err)
		return err
	}
	t.source.Status.Success()
	t.source.AddInput(t.path)

	ret, _ := f.Seek(offset, whence)
	t.file = f
	t.readOffset = ret
	t.decodedOffset = ret

	go t.readForever()
	return nil
}

func (t *Tailer) readForever() {
	for {
		if t.shouldHardStop() {
			t.onStop()
			return
		}

		inBuf := make([]byte, 4096)
		n, err := t.file.Read(inBuf)
		if err == io.EOF {
			if t.shouldSoftStop() {
				t.onStop()
				return
			}
			t.wait()
			continue
		}
		if err != nil {
			t.source.Status.Error(err)
			log.Error("Err: ", err)
			return
		}
		if n == 0 {
			t.wait()
			continue
		}
		t.d.InputChan <- decoder.NewInput(inBuf[:n])
		t.incrementReadOffset(n)
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
	if inode(stat1) != inode(stat2) {
		return true, nil
	}

	if stat1.Size() < t.GetReadOffset() {
		return true, nil
	}
	return false, nil
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
