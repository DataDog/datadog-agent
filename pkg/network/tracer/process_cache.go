// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"strings"
	"sync"

	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	lru "github.com/hashicorp/golang-lru"
)

var defaultFilteredEnvs = []string{
	"DD_ENV",
	"DD_VERSION",
	"DD_SERVICE",
}

type process struct {
	Pid         uint32
	Envs        map[string]string
	ContainerID string
	StartTime   int64
}

type processList []*process

// maxProcessListSize is the max size of a processList
const maxProcessListSize = 5

type processCache struct {
	sync.Mutex

	// cache of pid -> list of processes holds a list of processes
	// with the same pid but differing start times up to a max of
	// maxProcessListSize. this is used to determine the closest
	// match to a connection's tiimestamp
	cacheByPid map[uint32]processList
	// lru cache; keyed by (pid, start time)
	cache *lru.Cache
	// filteredEnvs contains environment variable names
	// that a process in the cache must have; empty filteredEnvs
	// means no filter, and any process can be inserted the cache
	filteredEnvs []string
}

type processCacheKey struct {
	pid       uint32
	startTime int64
}

func newProcessCache(maxProcs int, filteredEnvs []string) (*processCache, error) {
	pc := &processCache{filteredEnvs: filteredEnvs, cacheByPid: map[uint32]processList{}}
	var err error
	pc.cache, err = lru.NewWithEvict(maxProcs, func(key, value interface{}) {
		p := value.(*process)
		pl, _ := pc.cacheByPid[p.Pid]
		if pl = pl.remove(p); len(pl) == 0 {
			delete(pc.cacheByPid, p.Pid)
			return
		}

		pc.cacheByPid[p.Pid] = pl
	})

	if err != nil {
		return nil, err
	}

	return pc, nil
}

func (pc *processCache) handleProcessEvent(entry *smodel.ProcessCacheEntry) {
	var envs map[string]string
	if entry.EnvsEntry != nil {
		for _, v := range entry.EnvsEntry.Values {
			kv := strings.SplitN(v, "=", 2)
			k := kv[0]
			found := len(pc.filteredEnvs) == 0
			for _, r := range pc.filteredEnvs {
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
			if len(pc.filteredEnvs) > 0 && len(pc.filteredEnvs) == len(envs) {
				break
			}
		}
	}

	if len(envs) == 0 && len(pc.filteredEnvs) > 0 && entry.ContainerID == "" {
		return
	}

	pc.add(&process{Pid: entry.Pid, Envs: envs, ContainerID: entry.ContainerID, StartTime: entry.ExecTime.UnixNano()})
}

func (pc *processCache) add(p *process) {
	pc.Lock()
	defer pc.Unlock()

	pc.cache.Add(processCacheKey{pid: p.Pid, startTime: p.StartTime}, p)
	pl, _ := pc.cacheByPid[p.Pid]
	pc.cacheByPid[p.Pid] = pl.update(p)
}

func (pc *processCache) get(pid uint32, ts int64) (*process, bool) {
	pc.Lock()
	defer pc.Unlock()

	pl, _ := pc.cacheByPid[pid]
	if closest := pl.closest(ts); closest != nil {
		pc.cache.Get(processCacheKey{pid: closest.Pid, startTime: closest.StartTime})
		return closest, true
	}

	return nil, false
}

func (pc *processCache) dump() (interface{}, error) {
	pc.Lock()
	defer pc.Unlock()

	res := map[uint32]interface{}{}
	for pid, pl := range pc.cacheByPid {
		res[pid] = pl
	}

	return res, nil
}

func (pl processList) update(p *process) processList {
	for i := range pl {
		if pl[i].Pid == p.Pid && pl[i].StartTime == p.StartTime {
			pl[i] = p
			return pl
		}
	}

	if len(pl) == maxProcessListSize {
		pl = pl[1:]
	}

	return append(pl, p)
}

func (pl processList) remove(p *process) processList {
	for i := range pl {
		if pl[i] == p {
			return append(pl[:i], pl[i+1:]...)
		}
	}

	return pl
}

func (pl processList) closest(ts int64) *process {
	var closest *process
	for i := range pl {
		if ts < pl[i].StartTime {
			continue
		}

		if closest == nil {
			closest = pl[i]
			continue
		}

		if pl[i].StartTime > closest.StartTime {
			closest = pl[i]
		}
	}

	return closest
}
