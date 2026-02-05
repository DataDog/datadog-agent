// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package containers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	utilintern "github.com/DataDog/datadog-agent/pkg/util/intern"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

// readResolvConf reads and strips a process's resolv.conf.
// If the resolv.conf is missing, it returns "<missing>" instead of an error.
// It can return various OS errors when the PID stopped running, so it needs to be
// followed up by a call to isProcessStillRunning
func (r *resolvStripper) readResolvConf(entry *events.Process) (string, error) {
	rootPath := hostRoot()
	if entry.ContainerID != nil {
		rootPath = kernel.HostProc(strconv.Itoa(int(entry.Pid)), "root")
	}

	resolvConfPath := filepath.Join(rootPath, "etc/resolv.conf")

	resolvConf, err := r.stripResolvConfFilepath(resolvConfPath)
	if errors.Is(err, os.ErrNotExist) {
		// report no file. don't turn this into an error, since if the process exited,
		// that will be checked later by isProcessStillRunning
		return "<missing>", nil
	}
	if err != nil {
		return "", resolvConfReadError(resolvConfPath, err)
	}

	return resolvConf, nil
}

func (r *resolvStripper) stripResolvConfFilepath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}

	return r.stripResolvConf(int(stat.Size()), file)
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

func (cr *containerReader) isProcessStillRunningImpl(ctx context.Context, entry *events.Process) (bool, error) {
	_, err := process.NewProcessWithContext(ctx, int32(entry.Pid))
	if errIsProcessNotRunning(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("isProcessStillRunning failed to create NewProcessWithContext: %w", err)
	}
	return true, nil
}
