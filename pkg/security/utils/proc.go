// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package utils holds utils related files
package utils

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// Getpid returns the current process ID in the host namespace
func Getpid() uint32 {
	p, err := os.Readlink(kernel.HostProc("/self"))
	if err == nil {
		if pid, err := strconv.ParseInt(p, 10, 32); err == nil {
			return uint32(pid)
		}
	}
	return uint32(os.Getpid())
}

var networkNamespacePattern = regexp.MustCompile(`net:\[(\d+)\]`)

// NetNSPath represents a network namespace path
type NetNSPath struct {
	mu         sync.Mutex
	pid        uint32
	cachedPath string
}

// NetNSPathFromPid returns a new NetNSPath from the given Pid
func NetNSPathFromPid(pid uint32) *NetNSPath {
	return &NetNSPath{
		pid: pid,
	}
}

// NetNSPathFromPath returns a new NetNSPath from the given path
func NetNSPathFromPath(path string) *NetNSPath {
	return &NetNSPath{
		cachedPath: path,
	}
}

// GetPath returns the path for the given network namespace
func (path *NetNSPath) GetPath() string {
	path.mu.Lock()
	defer path.mu.Unlock()

	if path.cachedPath == "" {
		path.cachedPath = procPidPath(path.pid, "ns/net")
	}
	return path.cachedPath
}

// GetProcessNetworkNamespace returns the network namespace of a pid after parsing /proc/[pid]/ns/net
func (path *NetNSPath) GetProcessNetworkNamespace() (uint32, error) {
	// open netns
	f, err := os.Open(path.GetPath())
	if err != nil {
		return 0, err
	}
	defer f.Close()

	l, err := os.Readlink(f.Name())
	if err != nil {
		return 0, err
	}

	matches := networkNamespacePattern.FindSubmatch([]byte(l))
	if len(matches) <= 1 {
		return 0, fmt.Errorf("couldn't parse network namespace ID: %s", l)
	}

	netns, err := strconv.ParseUint(string(matches[1]), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(netns), nil
}

// CgroupTaskPath returns the path to the cgroup file of a pid in /proc
func CgroupTaskPath(tgid, pid uint32) string {
	return kernel.HostProc(strconv.FormatUint(uint64(tgid), 10), "task", strconv.FormatUint(uint64(pid), 10), "cgroup")
}

// ProcExePath returns the path to the exe file of a pid in /proc
func ProcExePath(pid uint32) string {
	return procPidPath(pid, "exe")
}

// StatusPath returns the path to the status file of a pid in /proc
func StatusPath(pid uint32) string {
	return procPidPath(pid, "status")
}

// ProcRootPath returns the path to the root directory of a pid in /proc
func ProcRootPath(pid uint32) string {
	return procPidPath(pid, "root")
}

// ProcRootFilePath returns the path to the input file after prepending the proc root path of the given pid
func ProcRootFilePath(pid uint32, file string) string {
	return filepath.Join(ProcRootPath(pid), file)
}

func procPidPath(pid uint32, path string) string {
	return kernel.HostProc(strconv.FormatUint(uint64(pid), 10), path)
}

// ModulesPath returns the path to the modules file in /proc
func ModulesPath() string {
	return kernel.HostProc("modules")
}

// CapEffCapEprm returns the effective and permitted kernel capabilities of a process
func CapEffCapEprm(pid uint32) (uint64, uint64, error) {
	var capEff, capPrm uint64
	contents, err := os.ReadFile(StatusPath(uint32(pid)))
	if err != nil {
		return 0, 0, err
	}
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		capKind, value, found := strings.Cut(line, "\t")
		if !found {
			continue
		}

		switch strings.TrimRight(capKind, ":") {
		case "CapEff":
			capEff, err = strconv.ParseUint(value, 16, 64)
			if err != nil {
				return 0, 0, err
			}
		case "CapPrm":
			capPrm, err = strconv.ParseUint(value, 16, 64)
			if err != nil {
				return 0, 0, err
			}
		}
	}
	return capEff, capPrm, nil
}

// PidTTY returns the TTY of the given pid
func PidTTY(pid uint32) string {
	fdPath := procPidPath(pid, "fd/0")

	ttyPath, err := os.Readlink(fdPath)
	if err != nil {
		return ""
	}

	if ttyPath == "/dev/null" {
		return ""
	}

	if strings.HasPrefix(ttyPath, "/dev/pts") {
		return "pts" + path.Base(ttyPath)
	}

	if strings.HasPrefix(ttyPath, "/dev") {
		return path.Base(ttyPath)
	}

	return ""
}

// GetProcesses returns list of active processes
func GetProcesses() ([]*process.Process, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	var processes []*process.Process
	for _, pid := range pids {
		var proc *process.Process
		proc, err = process.NewProcess(pid)
		if err != nil {
			// the process does not exist anymore, continue
			continue
		}
		processes = append(processes, proc)
	}

	return processes, nil
}

// FilledProcess defines a filled process
type FilledProcess struct {
	Pid        int32
	Ppid       int32
	CreateTime int64
	Name       string
	Uids       []int32
	Gids       []int32
	MemInfo    *process.MemoryInfoStat
	Cmdline    []string
}

// GetFilledProcess returns a FilledProcess from a Process input
func GetFilledProcess(p *process.Process) (*FilledProcess, error) {
	ppid, err := p.Ppid()
	if err != nil {
		return nil, err
	}

	createTime, err := p.CreateTime()
	if err != nil {
		return nil, err
	}

	uids, err := p.Uids()
	if err != nil {
		return nil, err
	}

	gids, err := p.Gids()
	if err != nil {
		return nil, err
	}

	name, err := p.Name()
	if err != nil {
		return nil, err
	}

	memInfo, err := p.MemoryInfo()
	if err != nil {
		return nil, err
	}

	cmdLine, err := p.CmdlineSlice()
	if err != nil {
		return nil, err
	}

	return &FilledProcess{
		Pid:        p.Pid,
		Ppid:       ppid,
		CreateTime: createTime,
		Name:       name,
		Uids:       uids,
		Gids:       gids,
		MemInfo:    memInfo,
		Cmdline:    cmdLine,
	}, nil
}

// MaxEnvVarsCollected defines the max env vars collected
const MaxEnvVarsCollected = 256

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

func newEnvScanner(f *os.File) (*bufio.Scanner, error) {
	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(f)
	scanner.Split(zeroSplitter)

	return scanner, nil
}

func matchesOnePrefix(text string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

// EnvVars returns a array with the environment variables of the given pid
func EnvVars(priorityEnvsPrefixes []string, pid uint32) ([]string, bool, error) {
	filename := procPidPath(pid, "environ")

	f, err := os.Open(filename)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	// first pass collecting only priority variables
	scanner, err := newEnvScanner(f)
	if err != nil {
		return nil, false, err
	}
	var priorityEnvs []string
	envCounter := 0

	for scanner.Scan() {
		text := scanner.Text()
		if len(text) > 0 {
			envCounter++
			if matchesOnePrefix(text, priorityEnvsPrefixes) {
				priorityEnvs = append(priorityEnvs, text)
			}
		}
	}

	if envCounter > MaxEnvVarsCollected {
		envCounter = MaxEnvVarsCollected
	}

	// second pass collecting
	scanner, err = newEnvScanner(f)
	if err != nil {
		return nil, false, err
	}
	envs := make([]string, 0, envCounter)
	envs = append(envs, priorityEnvs...)

	for scanner.Scan() {
		if len(envs) >= MaxEnvVarsCollected {
			return envs, true, nil
		}

		text := scanner.Text()
		if len(text) > 0 {
			// if it matches one prefix, it's already in the envs through priority envs
			if !matchesOnePrefix(text, priorityEnvsPrefixes) {
				envs = append(envs, text)
			}
		}
	}

	return envs, false, nil
}

// ProcFSModule is a representation of a line in /proc/modules
type ProcFSModule struct {
	// Name is the name of the module
	Name string
	// Size is the memory size of the module, in bytes
	Size int
	// InstancesCount lists how many instances of the module are currently loaded
	InstancesCount int
	// DependsOn lists the modules which the current module depends on
	DependsOn []string
	// State is the state which the current module is in
	State string
	// Address is the address at which the module was loaded
	Address int64
	// TaintState is the kernel taint state of the module
	TaintState string
}

// FetchLoadedModules returns a map of loaded modules
func FetchLoadedModules() (map[string]ProcFSModule, error) {
	procModules, err := os.ReadFile(ModulesPath())
	if err != nil {
		return nil, err
	}

	output := make(map[string]ProcFSModule)
	lines := strings.Split(string(procModules), "\n")
	for _, line := range lines {
		split := strings.Split(line, " ")
		if len(split) < 6 {
			continue
		}

		newModule := ProcFSModule{
			Name:  split[0],
			State: split[4],
		}

		if len(split) >= 7 {
			newModule.TaintState = split[6]
		}

		newModule.Size, err = strconv.Atoi(split[1])
		if err != nil {
			// set to 0 by default
			newModule.Size = 0
		}

		newModule.InstancesCount, err = strconv.Atoi(split[2])
		if err != nil {
			// set to 0 by default
			newModule.InstancesCount = 0
		}

		if split[3] != "-" {
			newModule.DependsOn = strings.Split(split[3], ",")
			// remove empty entry
			if len(newModule.DependsOn[len(newModule.DependsOn)-1]) == 0 {
				newModule.DependsOn = newModule.DependsOn[0 : len(newModule.DependsOn)-1]
			}
		}

		newModule.Address, err = strconv.ParseInt(strings.TrimPrefix(split[5], "0x"), 16, 64)
		if err != nil {
			// set to 0 by default
			newModule.Address = 0
		}

		output[newModule.Name] = newModule
	}

	return output, nil
}
