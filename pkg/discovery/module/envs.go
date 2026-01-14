// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/discovery/envs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// addEnvToMap splits a list of strings containing environment variables of the
// format NAME=VAL to a map.
func addEnvToMap(env string, envs map[string]string) {
	name, val, found := strings.Cut(env, "=")
	if found {
		envs[name] = val
	}
}

// getEnvs gets the environment variables for the process.
func getEnvs(proc *process.Process) (map[string]string, error) {
	procEnvs, err := proc.Environ()
	if err != nil {
		return nil, err
	}
	envs := make(map[string]string, len(procEnvs))
	for _, env := range procEnvs {
		addEnvToMap(env, envs)
	}
	return envs, nil
}

// EnvReader reads the environment variables from /proc/<pid>/environ file.
type EnvReader struct {
	file    *os.File       // open pointer to environment variables file
	scanner *bufio.Scanner // iterator to read strings from text file
	envs    envs.Variables // collected environment variables
}

func zeroSplitter(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i := 0; i < len(data); i++ {
		if data[i] == '\x00' {
			return i + 1, data[:i], nil
		}
	}
	if !atEOF {
		return 0, nil, nil
	}
	return 0, data, bufio.ErrFinalToken
}

// newEnvReader returns a new [EnvReader] to read from path, it reads null terminated strings.
func newEnvReader(proc *process.Process) (*EnvReader, error) {
	envPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "environ")
	file, err := os.Open(envPath)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	scanner.Split(zeroSplitter)

	return &EnvReader{
		file:    file,
		scanner: scanner,
		envs:    envs.Variables{},
	}, nil
}

// close closes an open file.
func (er *EnvReader) close() {
	if er.file != nil {
		er.file.Close()
	}
}

// add adds env. variable to the map of environment variables,
func (er *EnvReader) add() {
	env := er.scanner.Text()
	name, val, found := strings.Cut(env, "=")
	if found {
		er.envs.Set(name, val)
	}
}

// GetTargetEnvs reads the environment variables of interest from the /proc/<pid>/environ file.
func GetTargetEnvs(proc *process.Process) (envs.Variables, error) {
	reader, err := newEnvReader(proc)
	defer func() {
		if reader != nil {
			reader.close()
		}
	}()

	if err != nil {
		return envs.Variables{}, err
	}

	for reader.scanner.Scan() {
		reader.add()
	}

	return reader.envs, nil
}
