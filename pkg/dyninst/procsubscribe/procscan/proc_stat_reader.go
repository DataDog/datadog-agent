// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package procscan provides a scanner that discovers processes in the system
// using mechanisms used by service discovery.
package procscan

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// startTimeReader reads the start time of a process from the stat file.
//
// This structure is optimized because we parse these stat files very often.
type startTimeReader struct {
	pathBuf    bytes.Buffer
	procfsRoot string
	statBuf    []byte
}

func newStartTimeReader(procfsRoot string) *startTimeReader {
	return &startTimeReader{
		procfsRoot: procfsRoot,
		statBuf:    make([]byte, 4096),
	}
}

func (r *startTimeReader) read(pid int32) (uint64, error) {
	r.pathBuf.Reset()
	r.pathBuf.WriteString(r.procfsRoot)
	r.pathBuf.WriteString("/")
	pidStart := r.pathBuf.Len()
	n, _ := fmt.Fprintf(&r.pathBuf, "%d", pid)
	pidEnd := pidStart + n
	pidBytes := r.pathBuf.Bytes()[pidStart:pidEnd]
	r.pathBuf.WriteString("/stat")
	path := unsafe.String(
		unsafe.SliceData(r.pathBuf.Bytes()),
		r.pathBuf.Len(),
	)
	var fd int
	if err := ignoringEINTR(func() (err error) {
		fd, err = syscall.Open(path, syscall.O_RDONLY, 0)
		return err
	}); err != nil {
		return 0, fmt.Errorf("open %s: %w", r.pathBuf.String(), err)
	}
	defer func() {
		if err := ignoringEINTR(func() error {
			return syscall.Close(fd)
		}); err != nil {
			log.Errorf("startTimeReader: close %s: %v", r.pathBuf.String(), err)
		}
	}()
	if err := ignoringEINTR(func() (err error) {
		n, err = syscall.Read(fd, r.statBuf)
		return err
	}); err != nil {
		return 0, fmt.Errorf("read %s: %w", r.pathBuf.String(), err)
	}
	return startTimeTicksFromProcStat(r.statBuf[:n], pidBytes)
}

func ignoringEINTR(fn func() error) error {
	for {
		if err := fn(); !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

func startTimeTicksFromProcStat(
	buf []byte,
	pidBytes []byte,
) (uint64, error) {
	if !bytes.HasPrefix(buf, pidBytes) {
		return 0, fmt.Errorf("pid %s not found in stat file", pidBytes)
	}
	buf = buf[len(pidBytes):]
	commStart := bytes.IndexByte(buf, '(')
	if commStart == -1 {
		return 0, errors.New("comm not found in stat file")
	}
	buf = buf[commStart+1:]
	commEnd := bytes.LastIndexByte(buf, ')')
	if commEnd == -1 {
		return 0, errors.New("comm not found in stat file")
	}
	buf = buf[commEnd+1:]
	fieldIdx := 2                // we've read the pid and comm
	const starttimeFieldIdx = 22 // starttime is the 22nd field
	var fieldData []byte
	for f := range bytes.FieldsSeq(buf) {
		if fieldIdx++; fieldIdx == starttimeFieldIdx {
			fieldData = f
			break
		}
	}
	if len(fieldData) == 0 {
		return 0, errors.New("starttime not found in stat file")
	}

	starttime := unsafe.String(unsafe.SliceData(fieldData), len(fieldData))
	return strconv.ParseUint(starttime, 10, 64)
}
