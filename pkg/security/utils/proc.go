// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package utils

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/gopsutil/process"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// Getpid returns the current process ID in the host namespace
func Getpid() int32 {
	p, err := os.Readlink(filepath.Join(util.HostProc(), "/self"))
	if err == nil {
		if pid, err := strconv.ParseInt(p, 10, 32); err == nil {
			return int32(pid)
		}
	}
	return int32(os.Getpid())
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
		path.cachedPath = filepath.Join(util.HostProc(), fmt.Sprintf("%d/ns/net", path.pid))
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
	return util.HostProc(strconv.FormatUint(uint64(tgid), 10), "task", strconv.FormatUint(uint64(pid), 10), "cgroup")
}

// ProcExePath returns the path to the exe file of a pid in /proc
func ProcExePath(pid int32) string {
	return util.HostProc(strconv.FormatUint(uint64(pid), 10), "exe")
}

// StatusPath returns the path to the status file of a pid in /proc
func StatusPath(pid int32) string {
	return util.HostProc(strconv.FormatInt(int64(pid), 10), "status")
}

// ProcRootPath returns the path to the root directory of a pid in /proc
func ProcRootPath(pid int32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("%d/root", pid))
}

// ModulesPath returns the path to the modules file in /proc
func ModulesPath() string {
	return util.HostProc("modules")
}

// RootPath returns the path to the root folder of a pid in /proc
func RootPath(pid int32) string {
	return util.HostProc(strconv.FormatInt(int64(pid), 10), "root")
}

// CapEffCapEprm returns the effective and permitted kernel capabilities of a process
func CapEffCapEprm(pid int32) (uint64, uint64, error) {
	var capEff, capPrm uint64
	contents, err := os.ReadFile(StatusPath(pid))
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
func PidTTY(pid int32) string {
	fdPath := filepath.Join(util.HostProc(), fmt.Sprintf("%d/fd/0", pid))

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

// GetFilledProcess returns a FilledProcess from a Process input
// TODO: make a PR to export a similar function in Datadog/gopsutil. We only populate the fields we need for now.
func GetFilledProcess(p *process.Process) *process.FilledProcess {
	ppid, err := p.Ppid()
	if err != nil {
		return nil
	}

	createTime, err := p.CreateTime()
	if err != nil {
		return nil
	}

	uids, err := p.Uids()
	if err != nil {
		return nil
	}

	gids, err := p.Gids()
	if err != nil {
		return nil
	}

	name, err := p.Name()
	if err != nil {
		return nil
	}

	memInfo, err := p.MemoryInfo()
	if err != nil {
		return nil
	}

	cmdLine, err := p.CmdlineSlice()
	if err != nil {
		return nil
	}

	return &process.FilledProcess{
		Pid:        p.Pid,
		Ppid:       ppid,
		CreateTime: createTime,
		Name:       name,
		Uids:       uids,
		Gids:       gids,
		MemInfo:    memInfo,
		Cmdline:    cmdLine,
	}
}

// EnvVars returns a array with the environment variables of the given pid
func EnvVars(pid int32) ([]string, error) {
	filename := filepath.Join(util.HostProc(), fmt.Sprintf("/%d/environ", pid))

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	zero := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
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

	scanner := bufio.NewScanner(f)
	scanner.Split(zero)

	var envs []string
	for scanner.Scan() {
		text := scanner.Text()
		if len(text) > 0 {
			envs = append(envs, text)
		}
	}

	return envs, nil
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
