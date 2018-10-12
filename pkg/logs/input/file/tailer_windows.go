// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/decoder"
)

// setup sets up the file tailer
func (t *Tailer) setup(offset int64, whence int) error {
	path, err := filepath.Abs(t.path)
	if err != nil {
		return err
	}
	t.tags = []string{fmt.Sprintf("filename:%s", filepath.Base(t.path))}
	t.fullpath = path
	t.readOffset = offset
	t.decodedOffset = offset
	log.Info("Opening ", t.fullpath)
	return nil
}

func (t *Tailer) readAvailable() (err error) {
	err = nil
	f, err := openFile(t.fullpath)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err == nil {
		sz := st.Size()
		log.Debugf("Size is %d, offset is %d", sz, t.GetReadOffset())
		if sz == 0 {
			log.Debug("File size now zero, resetting offset")
			t.SetReadOffset(0)
			t.SetDecodedOffset(0)
		} else if sz < t.GetReadOffset() {
			log.Debug("Offset off end of file, resetting")
			t.SetReadOffset(0)
			t.SetDecodedOffset(0)
		}
	} else {
		log.Debugf("Error stat()ing file %v", err)
		return err
	}
	f.Seek(t.GetReadOffset(), io.SeekStart)

	for {
		inBuf := make([]byte, 4096)
		n, err := f.Read(inBuf)
		if n == 0 || err != nil {
			log.Debugf("Done reading")
			return err
		}
		log.Debugf("Sending %d bytes to input channel", n)
		t.decoder.InputChan <- decoder.NewInput(inBuf[:n])
		t.incrementReadOffset(n)
	}
}

// readForever lets the tailer tail the content of a file
// until it is closed.
func (t *Tailer) readForever() {
	defer t.onStop()
	for {
		select {
		case <-t.stop:
			// stop reading data from file
			return
		default:
			err := t.readAvailable()
			if err == io.EOF || os.IsNotExist(err) {
				t.wait()
				continue
			}
			if err != nil {
				t.source.Status.Error(err)
				log.Error("Err: ", err)
				return
			}
		}
	}
}

// openFile reimplement the os.Open function for Windows because the default
// implementation opens files without the FILE_SHARE_DELETE flag.
// cf: https://github.com/golang/go/blob/release-branch.go1.11/src/syscall/syscall_windows.go#L271
// This prevents users from moving/removing files when the tailer is reading the file.
// FIXME(achntrl): Should we stop opening/closing the file on every call to readAvailable ?
func openFile(path string) (*os.File, error) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	access := uint32(syscall.GENERIC_READ)
	// add FILE_SHARE_DELETE that is missing from os.Open implementation
	sharemode := uint32(syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE)
	createmode := uint32(syscall.OPEN_EXISTING)
	var sa *syscall.SecurityAttributes

	r, err := syscall.CreateFile(pathp, access, sharemode, sa, createmode, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(r), path), nil
}
