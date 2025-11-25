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

type containerReader struct {
	resolvStripper
}

type readContainerItemResult struct {
	item         containerStoreItem
	noDataReason string
}

func (cr *containerReader) readContainerItem(ctx context.Context, entry *events.Process) (readContainerItemResult, error) {
	resolvConf, err := cr.readResolvConf(entry)
	if err != nil {
		return readContainerItemResult{}, err
	}

	// we must check this last, to guarantee the result of readResolvConf is valid
	isRunning, err := isProcessStillRunning(ctx, entry)
	if err != nil {
		return readContainerItemResult{}, err
	}
	if !isRunning {
		return readContainerItemResult{
			noDataReason: "process not running",
		}, nil
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

func makeResolvStripper(size int) resolvStripper {
	return resolvStripper{
		buf: make([]byte, 0, size),
	}
}

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

func isProcessStillRunning(ctx context.Context, entry *events.Process) (bool, error) {
	proc, err := process.NewProcessWithContext(ctx, int32(entry.Pid))
	if errors.Is(err, process.ErrorProcessNotRunning) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("isProcessStillRunning failed to create NewProcessWithContext: %w", err)
	}

	createTime, err := proc.CreateTimeWithContext(ctx)
	if errors.Is(err, process.ErrorProcessNotRunning) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("isProcessStillRunning failed to get createTime: %w", err)
	}
	// StartTime is recorded as nanoseconds by security's EBPFResolver
	createTime *= int64(time.Millisecond)

	// detect (rare) PID reuse by comparing the StartTime
	if entry.StartTime != createTime {
		if log.ShouldLog(log.DebugLvl) {
			logDetectedProcessReuse(entry, createTime)
		}
		return false, nil
	}

	return true, nil
}

// logDetectedProcessReuse logs in a separate function to avoid allocation
func logDetectedProcessReuse(entry *events.Process, newTime int64) {
	log.Debugf("CNM ContainerStore detected process reuse on pid=%d: timestamps %d vs %d", entry.Pid, entry.StartTime, newTime)
}
