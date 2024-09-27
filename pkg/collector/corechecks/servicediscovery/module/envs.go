// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"bytes"
	"fmt"
	"hash/fnv"
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

const (
	// maxSizeEnvsMap - maximum number of returned environment variables
	maxSizeEnvsMap = 400
	// defSizeEnvsMap - default size for environment variables map
	defSizeEnvsMap = 200
	// defSizeReadBuf - default buffer size for reading from proc/pid/environ
	defSizeReadBuf = 2048
	// defSizeEnvBuf - default buffer size for storing processed environment variable
	defSizeEnvBuf = 100
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

// EnvScanner reads the environment variables file in chunks.
// Collects only those variables that match the target map if the map is not empty,
// otherwise collect all environment variables.
type EnvScanner struct {
	file    *os.File          // open pointer to environment variables file
	targets map[uint64]string // map of environment variables of interest
	buffer  []byte            // buffer for reading a fragment from a file
	end     int               // last index of data read
	err     error             // last detected error
	envs    map[string]string // collected environment variables
	env     []byte            // the last processed environment variable
}

// NewEnvScanner returns a new [EnvScanner] to read from path.
func NewEnvScanner(path string, bufSize int, targets []string) *EnvScanner {

	targetsMap := make(map[uint64]string)
	targetsNum := len(targets)

	if targetsNum == 0 {
		targetsNum = defSizeEnvsMap
	} else {
		for _, target := range targets {
			targetsMap[hashBytes([]byte(target))] = target
		}
	}

	file, err := os.Open(path)
	return &EnvScanner{
		file:    file,
		targets: targetsMap,
		buffer:  make([]byte, bufSize),
		end:     bufSize,
		err:     err,
		envs:    make(map[string]string, targetsNum),
		env:     make([]byte, 0, defSizeEnvBuf),
	}
}

// Finish closes an open file
func (es *EnvScanner) Finish() {
	if es.file != nil {
		es.file.Close()
	}
}

// hashBytes return hash value of a bytes array using FNV-1a hash function
func hashBytes(b []byte) uint64 {
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}

// addEnvToMapIfTarget add env. variable to map if it matches target or target map is empty.
func (es *EnvScanner) addEnvToMapIfTarget() {
	if len(es.envs) == maxSizeEnvsMap {
		es.err = fmt.Errorf("proc environment scanner can't add more than max (%d)", maxSizeEnvsMap)
		return
	}
	eq := bytes.IndexByte(es.env, '=')
	if eq == -1 {
		return
	}
	if len(es.targets) > 0 {
		h := hashBytes(es.env[:eq])
		_, exists := es.targets[h]
		if exists {
			name := string(es.env[:eq])
			es.envs[name] = string(es.env[eq+1:])
		}
	} else {
		name := string(es.env[:eq])
		es.envs[name] = string(es.env[eq+1:])
	}
}

// ScanFile reads a text file in chunks and extracts null-terminated strings,
// extracts the name and value of environment variables and stores them in a map.
// +-------------------------------+---------------+------------+-------------------+
// | read fragment may include     | end of string | equal sign | action to do      |
// |-------------------------------|---------------|------------|-------------------|
// | ^^^^^^[NULL]^^^^=^^^^^[NULL]  |     v         |     v      | save env.var      |
// | ^^^^^^[NULL]^^^^^^^^^^^^^^^^  |     v         |     -      | save env.var      |
// | ^^^^^^^^^^^^^^^^=^^^^^^^^^^^  |     -         |     v      | append to env.var |
// | ^^^^^^^^^^^^^^^^^^^^^^^^^^^^  |     -         |     -      | append to env.var |
// +-------------------------------+---------------+------------+-------------------+
func (es *EnvScanner) ScanFile() error {
	if len(es.envs) == maxSizeEnvsMap {
		es.err = fmt.Errorf("proc environment scanner reached maximum entries (%d)", maxSizeEnvsMap)
		return es.err
	}
	es.end, es.err = es.file.Read(es.buffer)
	if es.end <= 0 {
		return io.EOF
	}
	if es.err != nil {
		return es.err
	}
	cursor := 0 // indicates the beginning of the next string

	for cursor < es.end {
		strLen := bytes.IndexByte(es.buffer[cursor:], 0)
		strLen = min(strLen, es.end-cursor) // consider only bytes up to the end of the data end

		if strLen >= 0 {
			// zero length means NULL was encountered immediately.
			if strLen > 0 {
				es.env = append(es.env, es.buffer[cursor:cursor+strLen]...)
			}
			es.addEnvToMapIfTarget()
			if es.err != nil {
				return es.err
			}
			// reset processed env. var.
			es.env = es.env[:0]
			cursor += strLen + 1
		} else {
			es.env = append(es.env, es.buffer[cursor:es.end]...)
			break
		}
	}

	return nil
}

// getEnvironPath return path '/proc/<PID>/environ'
func getEnvironPath(proc *process.Process) string {
	hostProc := os.Getenv("HOST_PROC")
	if len(hostProc) == 0 {
		hostProc = "/proc"
	}
	return filepath.Join(hostProc, strconv.Itoa(int(proc.Pid)), "environ")
}

// getServiceEnvs searches the environment variables of interest in the proc file by reading the file in chunks.
func getServiceEnvs(proc *process.Process, buffSize int, onlyTargets bool) (map[string]string, error) {
	targets := targetEnvs
	if !onlyTargets {
		targets = nil
	}
	es := NewEnvScanner(getEnvironPath(proc), buffSize, targets)
	if es.err != nil {
		return nil, es.err
	}
	defer es.Finish()

	for {
		err := es.ScanFile()
		if err != nil {
			break
		}
	}
	if es.err != nil && es.err != io.EOF {
		return nil, es.err
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
