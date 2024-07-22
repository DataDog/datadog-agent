// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package uprobes

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const procFSUpdateTimeout = 10 * time.Millisecond

// ProcInfo holds the information extracted from procfs, to avoid repeat calls to the filesystem.
type ProcInfo struct {
	procRoot string
	PID      uint32
	exe      string
	comm     string
}

// NewProcInfo creates a new ProcInfo object.
func NewProcInfo(procRoot string, pid uint32) *ProcInfo {
	return &ProcInfo{
		procRoot: procRoot,
		PID:      pid,
	}
}

func waitUntilSucceeds[T any](p *ProcInfo, procFile string, readFunc func(string) (T, error)) (T, error) {
	// Read the exe link
	pidAsStr := strconv.FormatUint(uint64(p.PID), 10)
	filePath := filepath.Join(p.procRoot, pidAsStr, procFile)

	var result T
	err := errors.New("iteration start")
	end := time.Now().Add(procFSUpdateTimeout)

	for err != nil && end.After(time.Now()) {
		result, err = readFunc(filePath)
		if err != nil {
			time.Sleep(time.Millisecond)
		}
	}

	return result, nil
}

// Exe returns the path to the executable of the process.
func (p *ProcInfo) Exe() (string, error) {
	var err error
	if p.exe == "" {
		p.exe, err = waitUntilSucceeds(p, "exe", os.Readlink)
		if err != nil {
			return "", err
		}
	}

	return p.exe, nil
}

const (
	// Defined in https://man7.org/linux/man-pages/man5/proc.5.html.
	taskCommLen = 16
)

var (
	taskCommLenBufferPool = sync.Pool{
		New: func() any {
			buf := make([]byte, taskCommLen)
			return &buf
		},
	}
)

func (p *ProcInfo) readComm(commFile string) (string, error) {
	file, err := os.Open(commFile)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buf := taskCommLenBufferPool.Get().(*[]byte)
	defer taskCommLenBufferPool.Put(buf)
	n, err := file.Read(*buf)
	if err != nil {
		// short living process can hit here, or slow start of another process.
		return "", nil
	}
	return string(bytes.TrimSpace((*buf)[:n])), nil
}

// Comm returns the command name of the process.
func (p *ProcInfo) Comm() (string, error) {
	var err error
	if p.comm == "" {
		p.comm, err = waitUntilSucceeds(p, "comm", p.readComm)
		if err != nil {
			return "", err
		}
	}

	return p.comm, nil
}

var readBufferPool = ddsync.NewSlicePool[byte](128, 128)

func (p *ProcInfo) readCmdline(cmdlineFile string) (string, error) {
	file, err := os.Open(cmdlineFile)
	if err != nil {
		return "", err
	}
	defer file.Close()

	bufferPtr := readBufferPool.Get()
	defer readBufferPool.Put(bufferPtr)

	buffer := *bufferPtr
	n, _ := file.Read(buffer)
	if n == 0 {
		return "", nil
	}

	buffer = buffer[:n]
	i := bytes.Index(buffer, []byte{0})
	if i == -1 {
		return "", nil
	}

	return string(buffer[:i]), nil
}

func (p *ProcInfo) FindCmdlineWord(wordEnd []byte) (string, error) {
	f, err := waitUntilSucceeds(p, "cmdline", os.Open)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// From here on we shouldn't allocate for the common case
	// (eg., a process is *not* the pattern)
	bufferPtr := readBufferPool.Get()
	defer func() {
		readBufferPool.Put(bufferPtr)
	}()

	buffer := *bufferPtr
	n, _ := f.Read(buffer)
	if n == 0 {
		return "", nil
	}

	buffer = buffer[:n]
	i := bytes.Index(buffer, wordEnd)
	if i < 0 {
		return "", nil
	}

	executable := buffer[:i+len(wordEnd)]
	return string(executable), nil
}
