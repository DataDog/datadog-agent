// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model/sharedconsts"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// GetpidFrom returns the current process ID from the given proc root
func GetpidFrom(procRoot string) uint32 {
	p, err := os.Readlink(filepath.Join(procRoot, "self"))
	if err == nil {
		if pid, err := strconv.ParseInt(p, 10, 32); err == nil {
			return uint32(pid)
		}
	}
	return uint32(os.Getpid())
}

// Getpid returns the current process ID in the host namespace
func Getpid() uint32 {
	return GetpidFrom(kernel.ProcFSRoot())
}

var namespacePattern = regexp.MustCompile(`[a-z]+:\[(\d+)\]`)

// ErrNoNSPid is returned when no NSpid field is found in the status file, useful to distinguish between
// errors reading the file and the case where the field is not present, common for non-containerized processes.
var ErrNoNSPid = errors.New("no NSpid field found")

type NsType string

const (
	// cgroup
	CGroupNsType NsType = "cgroup"
	// ipc
	IpcNsType NsType = "ipc"
	// mnt
	MntNsType NsType = "mnt"
	// net
	NetNsType NsType = "net"
	// pid
	PidNsType NsType = "pid"
	// pid_for_children
	PidForChildrenNsType NsType = "pid_for_children"
	// time
	TimeNsType NsType = "time"
	// time_for_children
	TimeForChildrenNsType NsType = "time_for_children"
	// user
	UserNsType NsType = "user"
	// uts
	UtsNsType NsType = "uts"
)

// NSPath represents a network namespace path
type NSPath struct {
	mu         sync.Mutex
	pid        uint32
	cachedPath string
	nsType     NsType
}

// NewNSPathFromPid returns a new NSPath from the given Pid
func NewNSPathFromPid(pid uint32, nsType NsType) *NSPath {
	return &NSPath{
		pid:    pid,
		nsType: nsType,
	}
}

// NSPathFromPath returns a new NSPath from the given path
func NewNSPathFromPath(path string, nsType NsType) *NSPath {
	return &NSPath{
		cachedPath: path,
		nsType:     nsType,
	}
}

// GetPath returns the path for the given network namespace
func (path *NSPath) GetPath() string {
	path.mu.Lock()
	defer path.mu.Unlock()

	if path.cachedPath == "" {
		path.cachedPath = procPidPath(path.pid, "ns/"+string(path.nsType))
	}
	return path.cachedPath
}

// GetNSID returns the namespace ID of the given process
func (path *NSPath) GetNSID() (uint32, error) {
	return getNSIDFromPath(path.GetPath())
}

func getNSIDFromPath(path string) (uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	l, err := os.Readlink(f.Name())
	if err != nil {
		return 0, err
	}

	matches := namespacePattern.FindSubmatch([]byte(l))
	if len(matches) <= 1 {
		return 0, fmt.Errorf("couldn't parse namespace ID for path %s: %s", path, l)
	}

	ns, err := strconv.ParseUint(string(matches[1]), 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(ns), nil
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

// TaskStatusPath returns the path to the status file of a task pid in /proc
func TaskStatusPath(pid uint32, task string) string {
	return procPidPath3(pid, "task", task, "status")
}

// LoginUIDPath returns the path to the loginuid file of a pid in /proc
func LoginUIDPath(pid uint32) string {
	return procPidPath(pid, "loginuid")
}

// ProcRootPath returns the path to the root directory of a pid in /proc
func ProcRootPath(pid uint32) string {
	return procPidPath(pid, "root")
}

// ProcRootFilePath returns the path to the input file after prepending the proc root path of the given pid
func ProcRootFilePath(pid uint32, file string) string {
	// if file starts with /, the result of filepath.Join will look, before cleaning, like
	//   /proc/$PID/root//file
	// and this will require a re-allocation in filepath.Clean
	// to prevent this, we remove the leading / from the file if it's there. In most cases
	// it will be enough
	if file != "" && file[0] == os.PathSeparator {
		file = file[1:]
	}
	return procPidPath2(pid, "root", file)
}

// we do not use `HostProc` here because of the double call to `filepath.Join`
// and those functions can be called in a tight loop

func procPidPath(pid uint32, path string) string {
	return filepath.Join(kernel.ProcFSRoot(), strconv.FormatUint(uint64(pid), 10), path)
}

func procPidPath2(pid uint32, path1 string, path2 string) string {
	return filepath.Join(kernel.ProcFSRoot(), strconv.FormatUint(uint64(pid), 10), path1, path2)
}

func procPidPath3(pid uint32, path1, path2, path3 string) string {
	return filepath.Join(kernel.ProcFSRoot(), strconv.FormatUint(uint64(pid), 10), path1, path2, path3)
}

// ModulesPath returns the path to the modules file in /proc
func ModulesPath() string {
	return filepath.Join(kernel.ProcFSRoot(), "modules")
}

// GetLoginUID returns the login uid of the provided process
func GetLoginUID(pid uint32) (uint32, error) {
	content, err := os.ReadFile(LoginUIDPath(pid))
	if err != nil {
		return sharedconsts.AuditUIDUnset, err
	}

	data := strings.TrimSuffix(string(content), "\n")
	if len(data) == 0 {
		return sharedconsts.AuditUIDUnset, fmt.Errorf("invalid login uid: %v", data)
	}

	// parse login uid
	auid, err := strconv.ParseUint(data, 10, 32)
	if err != nil {
		return sharedconsts.AuditUIDUnset, fmt.Errorf("couldn't parse loginuid: %v", err)
	}
	return uint32(auid), nil
}

// CapEffCapEprm returns the effective and permitted kernel capabilities of a process
func CapEffCapEprm(pid uint32) (uint64, uint64, error) {
	var capEff, capPrm uint64
	contents, err := os.ReadFile(StatusPath(uint32(pid)))
	if err != nil {
		return 0, 0, err
	}
	lines := strings.SplitSeq(string(contents), "\n")
	for line := range lines {
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
func EnvVars(priorityEnvsPrefixes []string, pid uint32, maxEnvVars int) ([]string, bool, error) {
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

	if envCounter > maxEnvVars {
		envCounter = maxEnvVars
	}

	// second pass collecting
	scanner, err = newEnvScanner(f)
	if err != nil {
		return nil, false, err
	}
	envs := make([]string, 0, envCounter)
	envs = append(envs, priorityEnvs...)

	for scanner.Scan() {
		if len(envs) >= sharedconsts.MaxArgsEnvsSize {
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
	lines := strings.SplitSeq(string(procModules), "\n")
	for line := range lines {
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

// GetProcessPidNamespace returns the PID namespace of the given PID
func GetProcessPidNamespace(pid uint32) (uint64, error) {
	nspidPath := procPidPath(pid, "ns/pid")
	link, err := os.Readlink(nspidPath)
	if err != nil {
		return 0, err
	}
	// link should be in for of: pid:[4026532294]
	if !strings.HasPrefix(link, "pid:[") {
		return 0, fmt.Errorf("Failed to retrieve PID NS, pid ns malformed: (%s) err: %v", link, err)
	}

	link = strings.TrimPrefix(link, "pid:[")
	link = strings.TrimSuffix(link, "]")

	ns, err := strconv.ParseUint(link, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("Failed to retrieve PID NS, pid ns malformed: (%s) err: %v", link, err)
	}
	return ns, nil
}

// GetNsPids returns the namespaced pids of the given root pid
func GetNsPids(pid uint32, task string) ([]uint32, error) {
	statusFile := TaskStatusPath(pid, task)
	content, err := os.ReadFile(statusFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read status file: %w", err)
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		if after, ok := strings.CutPrefix(line, "NSpid:"); ok {
			// Remove "NSpid:" prefix and trim spaces
			values := after
			values = strings.TrimSpace(values)

			// Split the remaining string into fields
			fields := strings.Fields(values)

			// Convert string values to integers
			nspids := make([]uint32, 0, len(fields))
			for _, field := range fields {
				val, err := strconv.ParseUint(field, 10, 32)
				if err != nil {
					return nil, fmt.Errorf("failed to parse NSpid value: %w", err)
				}
				nspids = append(nspids, uint32(val))
			}
			return nspids, nil
		}
	}
	return nil, ErrNoNSPid
}

// GetPidTasks returns the task IDs of a process
func GetPidTasks(pid uint32) ([]string, error) {
	taskPath := procPidPath(pid, "task")

	// Read the contents of the task directory
	tasks, err := os.ReadDir(taskPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read task directory: %v", err)
	}

	// Collect all task IDs
	var taskIDs []string
	for _, task := range tasks {
		if task.IsDir() {
			taskIDs = append(taskIDs, task.Name())
		}
	}
	return taskIDs, nil
}

// FindPidNamespace search and return the host PID for the given namespaced PID + its namespace
func FindPidNamespace(nspid uint32, ns uint64) (uint32, error) {
	procPids, err := process.Pids()
	if err != nil {
		return 0, err
	}

	for _, procPid := range procPids {
		procNs, err := GetProcessPidNamespace(uint32(procPid))
		if err != nil {
			continue
		}

		if procNs != ns {
			continue
		}

		tasks, err := GetPidTasks(uint32(procPid))
		if err != nil {
			continue
		}

		for _, task := range tasks {
			nspids, err := GetNsPids(uint32(procPid), task)
			if err != nil {
				return 0, err
			}
			// we look only at the last one, as it the most inner one and corresponding to its /proc/pid/ns/pid namespace
			if nspids[len(nspids)-1] == nspid {
				return uint32(procPid), nil
			}
		}
	}
	return 0, errors.New("PID not found")
}

// GetTracerPid returns the tracer pid of the given root pid
func GetTracerPid(pid uint32) (uint32, error) {
	statusFile := StatusPath(pid)
	content, err := os.ReadFile(statusFile)
	if err != nil {
		return 0, fmt.Errorf("failed to read status file: %w", err)
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		if after, ok := strings.CutPrefix(line, "TracerPid:"); ok {
			// Remove "NSpid:" prefix and trim spaces
			line = after
			line = strings.TrimSpace(line)

			tracerPid, err := strconv.ParseUint(line, 10, 32)
			if err != nil {
				return 0, fmt.Errorf("failed to parse TracerPid value: %w", err)
			}
			return uint32(tracerPid), nil
		}
	}
	return 0, errors.New("TracerPid field not found")
}

// FindTraceesByTracerPid returns the process list being trced by the given tracer host PID
func FindTraceesByTracerPid(pid uint32) ([]uint32, error) {
	procPids, err := process.Pids()
	if err != nil {
		return nil, err
	}

	traceePids := []uint32{}
	for _, procPid := range procPids {
		tracerPid, err := GetTracerPid(uint32(procPid))
		if err != nil {
			continue
		}
		if tracerPid == pid {
			traceePids = append(traceePids, uint32(procPid))
		}
	}
	return traceePids, nil
}

var isNsPidAvailable = sync.OnceValue(func() bool {
	content, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}
	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		if strings.HasPrefix(line, "NSpid:") {
			return true
		}
	}
	return false
})

// TryToResolveTraceePid tries to resolve and returnt the HOST tracee PID, given the HOST tracer PID and the namespaced tracee PID.
func TryToResolveTraceePid(hostTracerPID uint32, tracerNSID uint64, NsTraceePid uint32) (uint32, error) {
	// Look if the NSpid status field is available or not (it should be, except for Centos7).
	if isNsPidAvailable() {
		/*
		   If it's available, we will search for an host pid having the same PID namespace as the
		   tracer, and having the corresponding NS PID in its status field
		*/
		pid, err := FindPidNamespace(NsTraceePid, tracerNSID)
		if err != nil {
			return 0, fmt.Errorf("Failed to resolve tracee PID namespace: %v", err)
		}
		return pid, nil
	}

	/*
	   Otherwise, we look at all process matching the tracer PID. And as a tracer can attach
	   to multiple tracees, we return a result only if we found only one.
	*/
	traceePids, err := FindTraceesByTracerPid(hostTracerPID)
	if err != nil {
		return 0, fmt.Errorf("Failed to find tracee pids matching tracer pid: %v", err)
	}
	if len(traceePids) == 1 {
		return traceePids[0], nil
	}

	return 0, errors.New("Unable to resolve host tracee PID")
}
