// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	injectorMemFdName = "dd_process_inject_info.msgpack"
	injectorMemFdPath = "/memfd:" + injectorMemFdName

	// memFdMaxSize is used to limit the amount of data we read from the memfd.
	// This is for safety to limit our memory usage in the case of a corrupt
	// file.
	// matches limit in the [auto injector](https://github.com/DataDog/auto_inject/blob/5ae819d01d8625c24dcf45b8fef32a7d94927d13/librouter.c#L52)
	memFdMaxSize = 65536
)

// GetInjectionMeta gets metadata from auto injector injection, if
// present. The auto injector creates a memfd file where it writes
// injection metadata such as injected environment variables, or versions
// of the auto injector and the library.
func GetInjectionMeta(proc *process.Process) (*InjectedProcess, bool) {
	path, found := findInjectorFile(proc)
	if !found {
		return nil, false
	}
	injectionMeta, err := extractInjectionMeta(path)
	if err != nil {
		log.Warnf("failed extracting injected envs: %s", err)
		return nil, false
	}
	return injectionMeta, true
}

func extractInjectionMeta(path string) (*InjectedProcess, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, memFdMaxSize))
	if err != nil {
		return nil, err
	}
	if len(data) == memFdMaxSize {
		return nil, io.ErrShortBuffer
	}

	var injectedProc InjectedProcess
	// Read data from injection
	guardrailsData, err := injectedProc.UnmarshalMsg(data)
	if err != nil {
		return nil, err
	}
	if len(guardrailsData) == 0 {
		return &injectedProc, nil
	}

	// If there are leftover bytes, they contain guardrails data
	_, err = injectedProc.UnmarshalMsg(guardrailsData)
	if err != nil {
		return nil, err
	}

	return &injectedProc, nil
}

// findInjectorFile searches for the injector file in the process open file descriptors.
// In order to find the correct file, we
// need to iterate the list of files (named after file descriptor numbers) in
// /proc/$PID/fd and get the name from the target of the symbolic link.
//
// ```
// $ ls -l /proc/1750097/fd/
// total 0
// lrwx------ 1 foo foo 64 Aug 13 14:24 0 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 1 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 2 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 3 -> '/dd_process_inject_info.msgpac (deleted)'
// ```
func findInjectorFile(proc *process.Process) (string, bool) {
	fdsPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "fd")
	// quick path, the shadow file is the first opened file by the process
	// unless there are inherited fds
	path := filepath.Join(fdsPath, "3")
	if isInjectorFile(path) {
		return path, true
	}
	fdDir, err := os.Open(fdsPath)
	if err != nil {
		log.Warnf("failed to open %s: %s", fdsPath, err)
		return "", false
	}
	defer fdDir.Close()
	fds, err := fdDir.Readdirnames(-1)
	if err != nil {
		log.Warnf("failed to read %s: %s", fdsPath, err)
		return "", false
	}
	for _, fd := range fds {
		switch fd {
		case "0", "1", "2", "3":
			continue
		default:
			path := filepath.Join(fdsPath, fd)
			if isInjectorFile(path) {
				return path, true
			}
		}
	}
	return "", false
}

func isInjectorFile(path string) bool {
	name, err := os.Readlink(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(name, injectorMemFdPath)
}

// addEnvToMap splits a list of strings containing environment variables of the
// format NAME=VAL to a map.
func addEnvToMap(env string, envs map[string]string) {
	name, val, found := strings.Cut(env, "=")
	if found {
		envs[name] = val
	}
}

// GetEnvs gets the environment variables for the process, both the initial
// ones, and if present, the ones injected via the auto injector.
func GetEnvs(proc *process.Process, injectionMeta *InjectedProcess) (map[string]string, error) {
	procEnvs, err := proc.Environ()
	if err != nil {
		return nil, err
	}
	envs := make(map[string]string, len(procEnvs))
	for _, env := range procEnvs {
		addEnvToMap(env, envs)
	}
	if injectionMeta == nil {
		return envs, nil
	}
	for _, env := range injectionMeta.InjectedEnv {
		addEnvToMap(string(env), envs)
	}
	return envs, nil
}
