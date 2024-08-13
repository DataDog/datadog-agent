// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package module

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	// injectorMemFdName is the name the injector (Datadog/auto_inject) uses.
	injectorMemFdName = "dd_environ"
	injectorMemFdPath = "/memfd:" + injectorMemFdName + " (deleted)"

	// memFdMaxSize is used to limit the amount of data we read from the memfd.
	// This is for safety to limit our memory usage in the case of a corrupt
	// file.
	memFdMaxSize = 4096
)

// readEnvsFile reads the env file created by the auto injector.
func readEnvsFile(path string) ([]string, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, memFdMaxSize))
	if err != nil {
		return nil, err
	}

	return strings.Split(string(data), "\000"), nil
}

// getInjectedEnvs gets environment variables injected by the auto injector, if
// present.
func getInjectedEnvs(proc *process.Process) []string {
	fdsPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "fd")
	entries, err := os.ReadDir(fdsPath)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		path := filepath.Join(fdsPath, entry.Name())
		name, err := os.Readlink(path)
		if err != nil {
			continue
		}

		if name != injectorMemFdPath {
			continue
		}

		envs, _ := readEnvsFile(path)
		return envs
	}

	return nil
}

// envsToMap splits a list of strings containing environment variables of the
// format NAME=VAL to a map.
func envsToMap(envs ...string) map[string]string {
	envMap := make(map[string]string, len(envs))
	for _, env := range envs {
		name, val, found := strings.Cut(env, "=")
		if !found {
			continue
		}

		envMap[name] = val
	}

	return envMap
}

// getEnvs gets the environment variables for the process, both the initial
// ones, and if present, the ones injected via the auto injector.
func getEnvs(proc *process.Process) (map[string]string, error) {
	envs, err := proc.Environ()
	if err != nil {
		return nil, nil
	}

	envs = append(envs, getInjectedEnvs(proc)...)
	return envsToMap(envs...), nil
}
