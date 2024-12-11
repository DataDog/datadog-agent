// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package ebpfcheck is the system-probe side of the eBPF check
package ebpfcheck

import (
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type mapProgHelperCache struct {
	sync.Mutex

	cache map[ebpf.MapID]helperProgData

	liveMapIDs map[ebpf.MapID]struct{}

	entryBtf    *btf.Func
	callbackBtf *btf.Func
}

func newMapProgHelperCache() *mapProgHelperCache {
	entryBtf := &btf.Func{
		Name: "entry",
		Type: &btf.FuncProto{
			Return: &btf.Int{
				Name:     "int",
				Size:     4,
				Encoding: btf.Signed,
			},
		},
	}

	u64Btf := &btf.Int{
		Name:     "long",
		Size:     8,
		Encoding: btf.Unsigned,
	}

	callbackBtf := &btf.Func{
		Name: "callback",
		Type: &btf.FuncProto{
			Return: u64Btf,
			Params: []btf.FuncParam{
				{
					Name: "map",
					Type: u64Btf,
				},
				{
					Name: "key",
					Type: u64Btf,
				},
				{
					Name: "value",
					Type: u64Btf,
				},
				{
					Name: "ctx",
					Type: u64Btf,
				},
			},
		},
	}

	return &mapProgHelperCache{
		cache:      make(map[ebpf.MapID]helperProgData),
		liveMapIDs: make(map[ebpf.MapID]struct{}),

		entryBtf:    entryBtf,
		callbackBtf: callbackBtf,
	}
}

func (c *mapProgHelperCache) newCachedProgramForMap(mp *ebpf.Map, mapid ebpf.MapID) (*ebpf.Program, error) {
	c.Lock()
	defer c.Unlock()

	if data, ok := c.cache[mapid]; ok {
		return data.prog, nil
	}

	data, err := c.newHelperProgramForFd(mp.FD())
	if err != nil {
		return nil, err
	}

	c.cache[mapid] = data
	return data.prog, nil
}

type helperProgData struct {
	id   ebpf.ProgramID
	prog *ebpf.Program
}

func (c *mapProgHelperCache) newHelperProgramForFd(fd int) (helperProgData, error) {
	/*
		equivalent to the following C code, based on the fact that `for_each_map_elem`
		returns the amount of entries visited, so visiting all the entries ensures we
		get the amount of entries in the map.

		int callback(struct bpf_map* map, const void* key, void* value, void * ctx) {
			return 0;
		}

		int entry() {
			return bpf_for_each_map_elem(fd, callback, NULL, 0);
		}
	*/

	spec := &ebpf.ProgramSpec{
		Type: ebpf.SocketFilter,
		Instructions: asm.Instructions{
			// entry
			btf.WithFuncMetadata(
				asm.LoadMapPtr(asm.R1, fd), // map fd
				c.entryBtf,
			),
			asm.Instruction{
				OpCode:   asm.LoadImmOp(asm.DWord),
				Src:      asm.PseudoFunc,
				Dst:      asm.R2,
				Constant: -1,
			}.WithReference("callback"),
			asm.LoadImm(asm.R3, 0, asm.DWord), // callback ctx
			asm.LoadImm(asm.R4, 0, asm.DWord), // flags
			asm.FnForEachMapElem.Call(),

			asm.Instruction{
				OpCode: asm.Exit.Op(asm.ImmSource),
			},

			// callback
			btf.WithFuncMetadata(
				asm.LoadImm(asm.R0, 0, asm.DWord).WithSymbol("callback"),
				c.callbackBtf,
			),
			asm.Instruction{
				OpCode: asm.Exit.Op(asm.ImmSource),
			},
		},
		License: "GPL",
	}

	prog, err := ebpf.NewProgramWithOptions(spec, ebpf.ProgramOptions{
		LogDisabled: true,
	})
	if err != nil {
		return helperProgData{}, err
	}

	var progid ebpf.ProgramID
	info, err := prog.Info()
	if err == nil {
		if id, ok := info.ID(); ok {
			ddebpf.AddIgnoredProgramID(id)
			progid = id
		}
	}

	return helperProgData{
		id:   progid,
		prog: prog,
	}, nil
}

func (c *mapProgHelperCache) Close() {
	if c == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	for _, data := range c.cache {
		if err := data.prog.Close(); err != nil {
			log.Warnf("failed to close helper program: %v", err)
		}
		ddebpf.RemoveIgnoredProgramID(data.id)
	}
	clear(c.cache)
}

func (c *mapProgHelperCache) clearLiveMapIDs() {
	if c == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	clear(c.liveMapIDs)
}

func (c *mapProgHelperCache) addLiveMapID(id ebpf.MapID) {
	if c == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	c.liveMapIDs[id] = struct{}{}
}

func (c *mapProgHelperCache) closeUnusedPrograms() {
	if c == nil {
		return
	}

	c.Lock()
	defer c.Unlock()

	for id, data := range c.cache {
		if _, alive := c.liveMapIDs[id]; !alive {
			// the mapping is no longer existent so we can safely remove the program
			if err := data.prog.Close(); err != nil {
				log.Warnf("failed to close helper program: %v", err)
			}
			ddebpf.RemoveIgnoredProgramID(data.id)
			delete(c.cache, id)
		}
	}
}
