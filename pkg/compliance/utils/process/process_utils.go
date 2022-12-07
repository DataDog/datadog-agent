// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CheckedProcess represents a process with potentially overridden fields
type CheckedProcess struct {
	inner        *process.Process
	pid          int32
	name         string
	exe          string
	cmdLineSlice []string
}

// NewCheckedProcess returns a new checked process, based on a real process object
func NewCheckedProcess(p *process.Process) *CheckedProcess {
	return &CheckedProcess{
		inner:        p,
		pid:          p.Pid,
		name:         "",
		cmdLineSlice: nil,
	}
}

// NewCheckedFakeProcess returns a new checked process, based on a fake object
func NewCheckedFakeProcess(pid int32, name string, cmdLineSlice []string) *CheckedProcess {
	return &CheckedProcess{
		inner:        nil,
		pid:          pid,
		name:         name,
		cmdLineSlice: cmdLineSlice,
	}
}

// Pid returns the pid stored in this checked process
func (p *CheckedProcess) Pid() int32 {
	return p.pid
}

// Name returns the name stored in this checked process
func (p *CheckedProcess) Name() string {
	if p.name != "" || p.inner == nil {
		return p.name
	}

	innerName, err := p.inner.Name()
	if err != nil {
		log.Warnf("failed to fetch process (pid=%d) name: %v", p.pid, err)
		return ""
	}
	p.name = innerName
	return innerName
}

// Exe returns the executable path stored in this checked process
func (p *CheckedProcess) Exe() string {
	if p.exe != "" || p.inner == nil {
		return p.exe
	}

	innerExe, err := p.inner.Exe()
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			log.Debugf("failed to fetch process (pid=%d) exe: %v", p.pid, err)
		} else {
			log.Warnf("failed to fetch process (pid=%d) exe: %v", p.pid, err)
		}
		return ""
	}
	p.exe = innerExe
	return innerExe
}

// CmdlineSlice returns the cmd line slice stored in this checked process
func (p *CheckedProcess) CmdlineSlice() []string {
	if p.cmdLineSlice != nil || p.inner == nil {
		return p.cmdLineSlice
	}

	innerCmdLine, err := p.inner.CmdlineSlice()
	if err != nil {
		log.Warnf("failed to fetch process (pid=%d) cmdline: %v", p.pid, err)
		return nil
	}
	p.cmdLineSlice = innerCmdLine
	return innerCmdLine
}

// Processes holds process info indexed per PID
type Processes map[int32]*CheckedProcess

const (
	// ProcessCacheKey is the global cache key to use for compliance processes
	ProcessCacheKey string = "compliance-processes"
)

var (
	// Fetcher is a singleton function to fetch the active processes
	Fetcher = fetchProcesses
)

// FindProcessesByName returned the list of processes with the specified name
func (p Processes) FindProcessesByName(name string) []*CheckedProcess {
	return p.FindProcesses(func(p *CheckedProcess) bool {
		return p.Name() == name
	})
}

// FindProcesses returned the list of processes matching a specific function
func (p Processes) FindProcesses(matchFunc func(*CheckedProcess) bool) []*CheckedProcess {
	var results = make([]*CheckedProcess, 0)
	for _, process := range p {
		if matchFunc(process) {
			results = append(results, process)
		}
	}

	return results
}

func fetchProcesses() (Processes, error) {
	inners, err := process.Processes()
	if err != nil {
		return nil, err
	}

	res := make(map[int32]*CheckedProcess, len(inners))
	for _, p := range inners {
		res[p.Pid] = NewCheckedProcess(p)
	}
	return res, nil
}

// GetProcesses returns all the processes in cache that do not exceed a specified duration
func GetProcesses(maxAge time.Duration) (Processes, error) {
	if value, found := cache.Cache.Get(ProcessCacheKey); found {
		return value.(Processes), nil
	}

	log.Debug("Updating process cache")
	rawProcesses, err := Fetcher()
	if err != nil {
		return nil, err
	}

	cache.Cache.Set(ProcessCacheKey, rawProcesses, maxAge)
	return rawProcesses, nil
}

// ParseProcessCmdLine parses command lines arguments into a map of flags and options.
// Parsing is far from being exhaustive, however for now it works sufficiently well
// for standard flag style command args.
func ParseProcessCmdLine(args []string) map[string]string {
	results := make(map[string]string, 0)
	pendingFlagValue := false

	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			parts := strings.SplitN(arg, "=", 2)

			// We have -xxx=yyy, considering the flag completely resolved
			if len(parts) == 2 {
				results[parts[0]] = parts[1]
			} else {
				results[parts[0]] = ""
				pendingFlagValue = true
			}
		} else {
			if pendingFlagValue {
				results[args[i-1]] = arg
			} else {
				results[arg] = ""
			}
		}
	}

	return results
}

// ValueFromProcessFlag returns the first process with the specified name and flag
func ValueFromProcessFlag(name string, flag string, cacheValidity time.Duration) (interface{}, error) {
	log.Debugf("Resolving value from process: %s, flag %s", name, flag)

	processes, err := GetProcesses(cacheValidity)
	if err != nil {
		return "", fmt.Errorf("unable to fetch processes: %w", err)
	}

	matchedProcesses := processes.FindProcessesByName(name)
	for _, mp := range matchedProcesses {
		flagValues := ParseProcessCmdLine(mp.CmdlineSlice())
		return flagValues[flag], nil
	}

	return "", fmt.Errorf("failed to find process: %s", name)
}
