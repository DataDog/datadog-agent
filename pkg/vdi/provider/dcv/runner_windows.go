// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package dcv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	dcvServiceName       = "dcvserver"
	dcvServerExecutable  = "dcvserver.exe"
	dcvCommandExecutable = "dcv.exe"
)

type commandRunner struct {
	mu                sync.Mutex
	executable        string
	resolveExecutable func() (string, error)
}

// NewCommandRunner returns a runner that discovers the DCV command from the
// registered DCV server service and executes it without a shell.
func NewCommandRunner() Runner {
	return &commandRunner{resolveExecutable: resolveDCVExecutable}
}

// Run executes one of the arguments selected by Collector.
func (r *commandRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if err := validateCommandArgs(args); err != nil {
		return nil, err
	}
	executable, err := r.executablePath()
	if err != nil {
		return nil, fmt.Errorf("resolve DCV command: %w", err)
	}

	command := exec.CommandContext(ctx, executable, args...)
	var output bytes.Buffer
	boundedOutput := &limitedWriter{writer: &output, remaining: maxOutputBytes + 1}
	command.Stdout = boundedOutput
	command.Stderr = boundedOutput
	if err := command.Run(); err != nil {
		return output.Bytes(), fmt.Errorf("dcv command failed: %w: %s", err, truncate(output.String(), 1024))
	}
	if output.Len() > maxOutputBytes {
		return nil, errors.New("dcv command output exceeded 1 MiB")
	}
	return output.Bytes(), nil
}

func (r *commandRunner) executablePath() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.executable != "" {
		return r.executable, nil
	}
	executable, err := r.resolveExecutable()
	if err != nil {
		return "", err
	}
	r.executable = executable
	return executable, nil
}

func resolveDCVExecutable() (string, error) {
	managerHandle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return "", fmt.Errorf("connect to Windows service manager: %w", err)
	}
	defer windows.CloseServiceHandle(managerHandle) //nolint:errcheck

	serviceName, err := windows.UTF16PtrFromString(dcvServiceName)
	if err != nil {
		return "", fmt.Errorf("encode DCV service name: %w", err)
	}
	serviceHandle, err := windows.OpenService(managerHandle, serviceName, windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return "", fmt.Errorf("open %q service: %w", dcvServiceName, err)
	}
	service := &mgr.Service{Name: dcvServiceName, Handle: serviceHandle}
	defer service.Close() //nolint:errcheck

	config, err := service.Config()
	if err != nil {
		return "", fmt.Errorf("query %q service configuration: %w", dcvServiceName, err)
	}
	executable, err := dcvExecutableFromServiceCommand(config.BinaryPathName)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(executable)
	if err != nil {
		return "", fmt.Errorf("stat DCV command %q: %w", executable, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("DCV command %q is not a regular file", executable)
	}
	return executable, nil
}

func dcvExecutableFromServiceCommand(commandLine string) (string, error) {
	args, err := windows.DecomposeCommandLine(commandLine)
	if err != nil {
		return "", fmt.Errorf("parse %q service command: %w", dcvServiceName, err)
	}
	if len(args) == 0 {
		return "", fmt.Errorf("%q service has an empty binary path", dcvServiceName)
	}

	serverExecutable := filepath.Clean(args[0])
	if !filepath.IsAbs(serverExecutable) {
		return "", fmt.Errorf("%q service binary path %q is not absolute", dcvServiceName, serverExecutable)
	}
	if !strings.EqualFold(filepath.Base(serverExecutable), dcvServerExecutable) {
		return "", fmt.Errorf("%q service binary path %q does not end in %s", dcvServiceName, serverExecutable, dcvServerExecutable)
	}
	return filepath.Join(filepath.Dir(serverExecutable), dcvCommandExecutable), nil
}

type limitedWriter struct {
	mu        sync.Mutex
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	requested := len(p)
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	if len(p) > 0 {
		_, _ = w.writer.Write(p)
		w.remaining -= len(p)
	}
	return requested, nil
}
