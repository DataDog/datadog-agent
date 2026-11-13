// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

package containers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	pathrs "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	utilintern "github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var hostRoot = funcs.MemoizeNoError(func() string {
	if v := os.Getenv("HOST_ROOT"); v != "" {
		return v
	}
	if env.IsContainerized() {
		if _, err := os.Stat("/host"); err == nil {
			return "/host"
		}
	}
	return "/"
})

var stringInterner = utilintern.NewStringInterner()

type resolvConfReader interface {
	readResolvConf(entry *events.Process) (string, error)
}

type containerReader struct {
	resolvConfReader
	isProcessStillRunning func(ctx context.Context, entry *events.Process) (bool, error)
}

func newContainerReader(reader resolvConfReader) containerReader {
	cr := containerReader{
		resolvConfReader: reader,
	}
	cr.isProcessStillRunning = cr.isProcessStillRunningImpl
	return cr
}

type readContainerItemResult struct {
	item         containerStoreItem
	noDataReason string
}

func (r readContainerItemResult) String() string {
	if r.noDataReason != "" {
		return fmt.Sprintf("noData(%s)", r.noDataReason)
	}
	if r.item.resolvConf == nil {
		return "empty"
	}
	return fmt.Sprintf("resolvConf(%d bytes)", len(r.item.resolvConf.Get()))
}

func (cr *containerReader) readContainerItem(ctx context.Context, entry *events.Process) (readContainerItemResult, error) {
	resolvConf, resolvConfErr := cr.readResolvConf(entry)
	// we must check this last, to guarantee the result of readResolvConf is valid
	isRunning, isRunningErr := cr.isProcessStillRunning(ctx, entry)
	if isRunningErr != nil {
		return readContainerItemResult{}, isRunningErr
	}
	if !isRunning {
		return readContainerItemResult{
			noDataReason: "process not running",
		}, nil
	}
	// isProcessStillRunning only checks that /proc/<pid> exists, which stays true for
	// a process that is exiting or is a zombie. Its root is already gone by then, so
	// there is nothing to read; don't cache that as a missing resolv.conf.
	if errors.Is(resolvConfErr, errProcessRootGone) {
		return readContainerItemResult{
			noDataReason: "process exiting",
		}, nil
	}

	// now that we know the PID was still running when we read resolv.conf, we can check its result
	if resolvConfErr != nil {
		return readContainerItemResult{}, resolvConfErr
	}

	item := containerStoreItem{
		timestamp: time.Now(),
	}
	if resolvConf != "" {
		item.resolvConf = stringInterner.GetString(resolvConf)
	}

	return readContainerItemResult{item: item}, nil
}

func resolvConfTooBig(kind string, size int) string {
	return fmt.Sprintf("<too big: kind=%s size=%d>", kind, size)
}

func resolvConfReadError(resolvConfPath string, err error) error {
	return fmt.Errorf("readResolvConf failed to read %s: %w", resolvConfPath, err)
}

type resolvStripper struct {
	buf []byte
}

func makeResolvStripper(size int) *resolvStripper {
	return &resolvStripper{
		buf: make([]byte, 0, size),
	}
}

// errProcessRootGone is returned by readResolvConf when the root directory of the
// process can't be opened because the process is exiting or has exited.
var errProcessRootGone = errors.New("process root directory is gone")

func isNotExistErr(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ESRCH)
}

// readResolvConf reads and strips a process's resolv.conf.
// If the resolv.conf is missing, it returns "<missing>" instead of an error.
// If the process root itself is gone (the process is exiting, or is a zombie), it
// returns errProcessRootGone.
// It can return various OS errors when the PID stopped running, so it needs to be
// followed up by a call to isProcessStillRunning
func (r *resolvStripper) readResolvConf(entry *events.Process) (string, error) {
	rootPath := hostRoot()
	if entry.ContainerID != nil {
		rootPath = kernel.HostProc(strconv.Itoa(int(entry.Pid)), "root")
	}

	// Open the root separately, so that "the process has no root any more" can be told
	// apart from "the root has no etc/resolv.conf". The kernel returns ENOENT for
	// /proc/<pid>/root once the task has released its fs_struct in do_exit(), while
	// /proc/<pid> stays visible until the task is reaped, so isProcessStillRunning
	// still reports such a process as running.
	rootDir, err := openRoot(rootPath)
	if isNotExistErr(err) {
		return "", errProcessRootGone
	}
	if err != nil {
		return "", resolvConfReadError(rootPath, err)
	}
	defer rootDir.Close()

	file, err := openResolvConf(rootDir)
	if isNotExistErr(err) {
		// The process may have exited between opening its root and the lookup, and
		// tearing down its mount namespace detaches the bind-mounted resolv.conf.
		// do_exit() releases the fs_struct before the namespaces, so if the root can
		// still be opened now, the namespace was intact during the lookup.
		if entry.ContainerID != nil {
			recheck, recheckErr := openRoot(rootPath)
			if isNotExistErr(recheckErr) {
				return "", errProcessRootGone
			}
			if recheckErr == nil {
				recheck.Close()
			}
		}
		// report no file. don't turn this into an error, since if the process exited,
		// that will be checked later by isProcessStillRunning
		return "<missing>", nil
	}
	if err != nil {
		return "", resolvConfReadError(rootPath+"/etc/resolv.conf", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}

	return r.stripResolvConf(int(stat.Size()), file)
}

// openRoot opens a root directory as an O_PATH handle, for use with openResolvConf.
func openRoot(rootPath string) (*os.File, error) {
	return os.OpenFile(rootPath, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
}

// openResolvConf opens etc/resolv.conf within root, respecting absolute symlinks using pathrs.
func openResolvConf(root *os.File) (*os.File, error) {
	handle, err := pathrs.OpenatInRoot(root, "etc/resolv.conf")
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	return pathrs.Reopen(handle, unix.O_RDONLY)
}

func (r *resolvStripper) stripResolvConf(size int, f io.Reader) (string, error) {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(r.buf, cap(r.buf))
	var sb strings.Builder

	if size >= cap(r.buf) {
		return resolvConfTooBig("input", size), nil
	}
	sb.Grow(size)

	for scanner.Scan() {
		trim := bytes.TrimSpace(scanner.Bytes())
		if len(trim) == 0 {
			continue
		}
		if trim[0] == ';' || trim[0] == '#' {
			continue
		}
		if sb.Len() != 0 {
			sb.WriteByte('\n')
		}
		sb.Write(trim)
	}
	if scanner.Err() != nil {
		return "", scanner.Err()
	}

	resolvConf := sb.String()

	// limit the size of payload sent, and give the intake some statistics
	if len(resolvConf) > resolvConfMaxSizeBytes {
		resolvConf = resolvConfTooBig("output", len(resolvConf))
	}

	return resolvConf, nil
}

// errIsProcessNotRunning checks if an error is a process not running error
// gopsutil returns various errors when the process is not running, so we need to check for them all
func errIsProcessNotRunning(err error) bool {
	return errors.Is(err, process.ErrorProcessNotRunning) ||
		errors.Is(err, os.ErrProcessDone) ||
		errors.Is(err, os.ErrNotExist)
}

func (cr *containerReader) isProcessStillRunningImpl(ctx context.Context, entry *events.Process) (running bool, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("Recovered panic in isProcessStillRunningImpl for pid %d (likely malformed /proc stat): %v", entry.Pid, r)
			running = false
			retErr = nil
		}
	}()
	_, err := process.NewProcessWithContext(ctx, int32(entry.Pid))
	if errIsProcessNotRunning(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("isProcessStillRunning failed to create NewProcessWithContext: %w", err)
	}
	return true, nil
}
