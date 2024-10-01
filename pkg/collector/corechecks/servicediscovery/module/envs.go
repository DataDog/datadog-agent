// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"bufio"
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
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

const (
	// defSizeEnvsMap - default size for environment variables map
	defSizeEnvsMap = 200
	// maxSizeEnvsMap - maximum number of returned environment variables
	maxSizeEnvsMap = 400
	// envVarDdService - name of target environment variable DD_SERVICE
	envVarDdService = "DD_SERVICE"
	// envVarDdTags - name of target environment variable DD_TAGS
	envVarDdTags = "DD_TAGS"
	// envVarDdInjectionEnabled - name of target environment variable DD_INJECTION_ENABLED
	envVarDdInjectionEnabled = "DD_INJECTION_ENABLED"
	// envVarDiscoveryEnabled - name of target environment variable DD_DISCOVERY_ENABLED
	envVarDiscoveryEnabled = "DD_DISCOVERY_ENABLED"
	// envVarOtelServiceName - name of target environment variable OTEL_SERVICE_NAME
	envVarOtelServiceName = "DD_OTEL_SERVICE_NAME"
)

// map of environment variables of interest
var targetEnvs = []string{
	envVarDdService,
	envVarDdTags,
	envVarDdInjectionEnabled,
	envVarDiscoveryEnabled,
	envVarOtelServiceName,
}

// getInjectionMeta gets metadata from auto injector injection, if
// present. The auto injector creates a memfd file where it writes
// injection metadata such as injected environment variables, or versions
// of the auto injector and the library.
func getInjectionMeta(proc *process.Process) (*InjectedProcess, bool) {
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
	if _, err = injectedProc.UnmarshalMsg(data); err != nil {
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

// getEnvs gets the environment variables for the process, both the initial
// ones, and if present, the ones injected via the auto injector.
func getEnvs(proc *process.Process) (map[string]string, error) {
	procEnvs, err := proc.Environ()
	if err != nil {
		return nil, err
	}
	envs := make(map[string]string, len(procEnvs))
	for _, env := range procEnvs {
		addEnvToMap(env, envs)
	}
	injectionMeta, ok := getInjectionMeta(proc)
	if !ok {
		return envs, nil
	}
	for _, env := range injectionMeta.InjectedEnv {
		addEnvToMap(string(env), envs)
	}
	return envs, nil
}

// EnvScanner reads the environment variables from /ptoc file.
// It collects only those variables that match the target map if the map is not empty,
// otherwise collect all environment variables.
type EnvScanner struct {
	file    *os.File          // open pointer to environment variables file
	scanner *bufio.Scanner    // scanner to read file data
	targets map[uint64]string // map of environment variables of interest
	envs    map[string]string // collected environment variables
}

// hashBytes return hash value of a bytes array using FNV-1a hash function
func hashBytes(b []byte) uint64 {
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}

func buildTargetsMap() map[uint64]string {
	targetsMap := make(map[uint64]string)

	for _, target := range targetEnvs {
		targetsMap[hashBytes([]byte(target))] = target
	}
	for _, target := range apm.JavaDetectorEnvs {
		targetsMap[hashBytes([]byte(target))] = target
	}
	for _, target := range apm.DotNetDetectorEnvs {
		targetsMap[hashBytes([]byte(target))] = target
	}
	return targetsMap
}

// nullTerminatedString used to tokenize the scanner data
func nullTerminatedString(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[0:i], nil
	}
	if atEOF && len(data) > 0 {
		// end of the file, return the remaining data
		return len(data), data, nil
	}

	return 0, nil, nil
}

// newEnvScanner returns a new [EnvScanner] to read from path.
func newEnvScanner(proc *process.Process, onlyTargets bool) (*EnvScanner, error) {
	envPath := kernel.HostProc(strconv.Itoa(int(proc.Pid)), "environ")
	file, err := os.Open(envPath)
	if err != nil {
		return nil, err
	}
	var targetsMap map[uint64]string
	if onlyTargets {
		targetsMap = buildTargetsMap()
	}

	envsNum := len(targetsMap)
	if envsNum == 0 {
		envsNum = defSizeEnvsMap
	}

	scanner := bufio.NewScanner(file)
	scanner.Split(nullTerminatedString)

	return &EnvScanner{
		file:    file,
		scanner: scanner,
		targets: targetsMap,
		envs:    make(map[string]string, envsNum),
	}, nil
}

// Finish closes an open file
func (es *EnvScanner) finish() {
	if es.file != nil {
		es.file.Close()
	}
}

// addTargetEnvToMap add env. variable to the map of environment variables.
func (es *EnvScanner) addTargetEnvToMap() error {
	if len(es.envs) == maxSizeEnvsMap {
		return fmt.Errorf("proc scanner can't add more than max (%d) environment variables", maxSizeEnvsMap)
	}
	b := es.scanner.Bytes()
	eq := bytes.IndexByte(b, '=')
	if eq == -1 {
		// ignore invalid env. variable
		return nil
	}
	if len(es.targets) > 0 {
		h := hashBytes(b[:eq])
		_, exists := es.targets[h]
		if exists {
			name := string(b[:eq])
			es.envs[name] = string(b[eq+1:])
		}
	} else {
		name := string(b[:eq])
		es.envs[name] = string(b[eq+1:])
	}
	return nil
}

// getEnvsUsingScan searches the environment variables of interest in the proc file by reading the proc file
func getEnvsUsingScan(proc *process.Process, onlyTargets bool) (map[string]string, error) {
	es, err := newEnvScanner(proc, onlyTargets)
	if err != nil {
		return nil, err
	}
	defer es.finish()

	for es.scanner.Scan() {
		err := es.addTargetEnvToMap()
		if err != nil {
			return es.envs, err
		}
	}
	if err := es.scanner.Err(); err != nil {
		return es.envs, err
	}
	injectionMeta, ok := getInjectionMeta(proc)
	if !ok {
		return es.envs, nil
	}
	for _, env := range injectionMeta.InjectedEnv {
		addEnvToMap(string(env), es.envs)
	}
	return es.envs, nil
}
