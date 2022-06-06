// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"strings"

	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	lru "github.com/hashicorp/golang-lru"
)

var filteredEnvs = []string{
	"DD_ENV",
	"DD_VERSION",
	"DD_SERVICE",
}

// Process stores process info
type Process struct {
	Pid         uint32
	Envs        map[string]string
	ContainerId string
}

type processCache struct {
	cache *lru.Cache
}

func newProcessCache(maxProcs int) (*processCache, error) {
	cache, err := lru.New(maxProcs)
	if err != nil {
		return nil, err
	}

	return &processCache{cache: cache}, nil
}

func (pc *processCache) handleProcessEvent(entry *smodel.ProcessCacheEntry) {
	var envs map[string]string
	if entry.EnvsEntry != nil {
		for _, v := range entry.EnvsEntry.Values {
			kv := strings.SplitN(v, "=", 2)
			k := kv[0]
			found := len(filteredEnvs) == 0
			for _, r := range filteredEnvs {
				if k == r {
					found = true
					break
				}
			}
			if !found {
				continue
			}

			v = ""
			if len(kv) > 1 {
				v = kv[1]
			}

			if envs == nil {
				envs = make(map[string]string)
			}
			envs[k] = v
			if len(filteredEnvs) > 0 && len(filteredEnvs) == len(envs) {
				break
			}
		}
	}

	if len(envs) == 0 && len(filteredEnvs) > 0 && entry.ContainerID == "" {
		return
	}

	pc.cache.Add(entry.Pid, &Process{Pid: entry.Pid, Envs: envs, ContainerId: entry.ContainerID})
}

// Process looks up a process by PID
func (pc *processCache) Process(pid uint32) (*Process, bool) {
	p, ok := pc.cache.Get(pid)
	if ok {
		return p.(*Process), true
	}

	return nil, false
}

func (pc *processCache) dump() (interface{}, error) {
	keys := pc.cache.Keys()
	if len(keys) == 0 {
		return []interface{}{}, nil
	}

	var res []interface{}
	for _, k := range keys {
		p, ok := pc.cache.Peek(k)
		if !ok {
			continue
		}
		res = append(res, *(p.(*Process)))
	}

	return res, nil
}
