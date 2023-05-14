// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	// processCacheMaxAge is the cache TTL used for caching process tables by name
	processCacheMaxAge = 5 * time.Minute
)

var (
	processTablesCacheMu sync.Mutex
	processTablesCache   *simplelru.LRU[string, Processes]
)

func init() {
	processTablesCache, _ = simplelru.NewLRU[string, Processes](16, nil)
}

// FetchProcessesWithName scans the the list of processes with the specified name.
// Overridable for tests and mocks purposes.
var FetchProcessesWithName = defaultFetchProcessesWithName

// IsProcessMetadataValid returns whether or not the cached ProcessMetadata is in sync
// with the system proc table. By default it will checks that a process with the
// same PID still exists and that the process create time is the same.
// Overridable for tests and mocks purposes.
var IsProcessMetadataValid = defaultIsProcessMetadataValid

// Processes is a list of ProcessMetadata
type Processes []*ProcessMetadata

// ProcessMetadata holds the metadata of a process required by our compliance rules. It is
// used as a caching layer to avoid having to fetch the system processes on every rule
// evaluation.
type ProcessMetadata struct {
	Pid        int32
	CreateTime int64
	Name       string
	Cmdline    []string
	Envs       []string

	cacheTime time.Time
}

// CmdlineFlags parses command lines arguments into a map of flags and options.
// Parsing is far from being exhaustive, however for now it works sufficiently well
// for standard flag style command args.
func (p *ProcessMetadata) CmdlineFlags() map[string]string {
	return parseCmdLineFlags(p.Cmdline)
}

// EnvsMap returns a map of the requested environment variables in this checked process
func (p *ProcessMetadata) EnvsMap(filteredEnvs []string) map[string]string {
	envsMap := make(map[string]string, len(filteredEnvs))
	if len(filteredEnvs) == 0 {
		return envsMap
	}
	for _, envValue := range p.Envs {
		for _, envName := range filteredEnvs {
			prefix := envName + "="
			if strings.HasPrefix(envValue, prefix) {
				envsMap[envName] = strings.TrimPrefix(envValue, prefix)
			} else if envValue == envName {
				envsMap[envName] = ""
			}
		}
	}
	return envsMap
}

// NewProcessMetadata returns a new ProcessMetadata struct.
func NewProcessMetadata(pid int32, createTime int64, name string, cmdline []string, envs []string) *ProcessMetadata {
	return &ProcessMetadata{
		Pid:        pid,
		CreateTime: createTime,
		Name:       name,
		Cmdline:    cmdline,
		Envs:       envs,

		cacheTime: time.Now(),
	}
}

func defaultFetchProcessesWithName(searchedName string) (Processes, error) {
	pids, err := process.Pids()
	if err != nil {
		return nil, err
	}
	var table Processes
	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		name, err := p.Name()
		if err != nil || name != searchedName {
			continue
		}
		createTime, err := p.CreateTime()
		if err != nil {
			return nil, err
		}
		cmdline, err := p.CmdlineSlice()
		if err != nil {
			return nil, err
		}
		envs, err := p.Environ()
		// NOTE(pierre): security-agent may be executed without the capabilities to get /proc/<pid>/environ
		if err != nil && !os.IsPermission(err) {
			return nil, err
		}
		table = append(table, NewProcessMetadata(pid, createTime, name, cmdline, envs))
	}
	return table, nil
}

func defaultIsProcessMetadataValid(p *ProcessMetadata) bool {
	if time.Since(p.cacheTime) > processCacheMaxAge {
		return false
	}
	proc, err := process.NewProcess(p.Pid)
	if err != nil {
		return false
	}
	createTime, err := proc.CreateTime()
	if err != nil {
		return false
	}
	return createTime == p.CreateTime
}

func parseCmdLineFlags(cmdline []string) map[string]string {
	flagsMap := make(map[string]string, 0)
	pendingFlagValue := false
	for i, arg := range cmdline {
		if strings.HasPrefix(arg, "-") {
			parts := strings.SplitN(arg, "=", 2)
			// We have -xxx=yyy, considering the flag completely resolved
			if len(parts) == 2 {
				flagsMap[parts[0]] = parts[1]
			} else {
				flagsMap[parts[0]] = ""
				pendingFlagValue = true
			}
		} else {
			if pendingFlagValue {
				flagsMap[cmdline[i-1]] = arg
			} else {
				flagsMap[arg] = ""
			}
		}
	}
	return flagsMap
}

// FindProcessesByName returns a list of *ProcessMetadata matching the given name.
func FindProcessesByName(searchedName string) (Processes, error) {
	processTablesCacheMu.Lock()
	defer processTablesCacheMu.Unlock()
	processTableMatching, ok := processTablesCache.Get(searchedName)
	if ok {
		for _, p := range processTableMatching {
			if !IsProcessMetadataValid(p) {
				ok = false
				break
			}
		}
	}
	if !ok {
		processTable, err := FetchProcessesWithName(searchedName)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch processes: %w", err)
		}
		processTablesCache.Add(searchedName, processTable)
		processTableMatching = processTable
	}
	return processTableMatching, nil
}

// PurgeCache cleans up the process table cache.
func PurgeCache() {
	processTablesCacheMu.Lock()
	defer processTablesCacheMu.Unlock()
	processTablesCache.Purge()
}

// ValueFromProcessFlag returns the first process with the specified name and flag
func ValueFromProcessFlag(name, flag string) (interface{}, error) {
	log.Debugf("Resolving value from process: %s, flag %s", name, flag)
	matchedProcesses, err := FindProcessesByName(name)
	if err != nil {
		return nil, err
	}
	if len(matchedProcesses) == 0 {
		return "", fmt.Errorf("failed to find process: %s", name)
	}
	flagValues := matchedProcesses[0].CmdlineFlags()
	return flagValues[flag], nil
}
