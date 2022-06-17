// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type CheckedProcess struct {
	inner        *process.Process
	pid          int32
	name         string
	exe          string
	cmdLineSlice []string
}

func NewCheckedProcess(p *process.Process) *CheckedProcess {
	return &CheckedProcess{
		inner:        p,
		pid:          p.Pid,
		name:         "",
		cmdLineSlice: nil,
	}
}

func NewCheckedFakeProcess(pid int32, name string, cmdLineSlice []string) *CheckedProcess {
	return &CheckedProcess{
		inner:        nil,
		pid:          pid,
		name:         name,
		cmdLineSlice: cmdLineSlice,
	}
}

func (p *CheckedProcess) Pid() int32 {
	return p.pid
}

func (p *CheckedProcess) Name() (string, error) {
	if p.name != "" || p.inner == nil {
		return p.name, nil
	}

	innerName, err := p.inner.Name()
	if err != nil {
		return "", err
	}
	p.name = innerName
	return innerName, nil
}

func (p *CheckedProcess) Exe() (string, error) {
	if p.exe != "" || p.inner == nil {
		return p.exe, nil
	}

	innerExe, err := p.inner.Exe()
	if err != nil {
		return "", err
	}
	p.exe = innerExe
	return innerExe, nil
}

func (p *CheckedProcess) CmdlineSlice() ([]string, error) {
	if p.cmdLineSlice != nil || p.inner == nil {
		return p.cmdLineSlice, nil
	}

	innerCmdLine, err := p.inner.CmdlineSlice()
	if err != nil {
		return nil, err
	}
	p.cmdLineSlice = innerCmdLine
	return innerCmdLine, nil
}

type processes []*CheckedProcess

const (
	processCacheKey string = "compliance-processes"
)

var (
	processFetcher = fetchProcesses
)

func (p processes) findProcessesByName(name string) []*CheckedProcess {
	return p.findProcesses(func(p *CheckedProcess) bool {
		pname, err := p.Name()
		if err != nil {
			return false
		}
		return pname == name
	})
}

func (p processes) findProcesses(matchFunc func(*CheckedProcess) bool) []*CheckedProcess {
	var results = make([]*CheckedProcess, 0)
	for _, process := range p {
		if matchFunc(process) {
			results = append(results, process)
		}
	}

	return results
}

func fetchProcesses() (processes, error) {
	inners, err := process.Processes()
	if err != nil {
		return nil, err
	}

	res := make([]*CheckedProcess, 0, len(inners))
	for _, p := range inners {
		res = append(res, NewCheckedProcess(p))
	}
	return res, nil
}

func getProcesses(maxAge time.Duration) (processes, error) {
	if value, found := cache.Cache.Get(processCacheKey); found {
		return value.(processes), nil
	}

	log.Debug("Updating process cache")
	rawProcesses, err := processFetcher()
	if err != nil {
		return nil, err
	}

	cache.Cache.Set(processCacheKey, rawProcesses, maxAge)
	return rawProcesses, nil
}

// Parsing is far from being exhaustive, however for now it works sufficiently well
// for standard flag style command args.
func parseProcessCmdLine(args []string) map[string]string {
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
