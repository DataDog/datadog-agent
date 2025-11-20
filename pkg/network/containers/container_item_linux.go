// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package containers

import (
	"context"
	"errors"
	"fmt"
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

var errProcessNotRunning = &containerItemNoDataError{Err: fmt.Errorf("process is no longer running")}

func readContainerItem(ctx context.Context, entry *events.Process) (containerStoreItem, error) {
	resolvConf, err := readResolvConf(entry)
	if err != nil {
		return containerStoreItem{}, err
	}

	// we must check this last, to guarantee the result of readResolvConf is valid
	isRunning, err := isProcessStillRunning(ctx, entry)
	if err != nil {
		return containerStoreItem{}, err
	}
	if !isRunning {
		return containerStoreItem{}, errProcessNotRunning
	}

	// limit the size of payload sent, and give the intake some statistics
	if len(resolvConf) > resolvConfMaxSizeBytes {
		resolvConf = fmt.Sprintf("<too big: %d>", len(resolvConf))
	}

	item := containerStoreItem{
		timestamp:  time.Now(),
		resolvConf: stringInterner.GetString(resolvConf),
	}

	return item, nil
}

func readResolvConf(entry *events.Process) (string, error) {
	rootPath := hostRoot()
	if entry.ContainerID != nil {
		rootPath = kernel.HostProc(strconv.Itoa(int(entry.Pid)), "root")
	}

	resolvConfPath := filepath.Join(rootPath, "etc/resolv.conf")
	data, err := os.ReadFile(resolvConfPath)
	if errors.Is(err, os.ErrNotExist) {
		// report no data. don't turn this into an error, since if the process exited
		// that will be checked later by isProcessStillRunning
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("readResolvConf failed to read %s: %w", resolvConfPath, err)
	}

	resolvConf := StripResolvConf(string(data))

	return resolvConf, nil
}

// StripResolvConf removes comments from resolv.conf
func StripResolvConf(resolvConf string) string {
	lines := strings.Split(resolvConf, "\n")
	var sb strings.Builder
	sb.Grow(len(resolvConf))

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if len(line) == 0 {
			continue
		}
		if line[0] == ';' || line[0] == '#' {
			continue
		}
		sb.WriteString(line)
	}

	return sb.String()
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
		log.Debugf("CNM ContainerStore detected process reuse on pid=%d: timestamps %d vs %d", entry.Pid, entry.StartTime, createTime)
		return false, nil
	}

	return true, nil
}
