// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpfcheck

import (
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

var mapNameMapping = make(map[uint32]string)
var mapModuleMapping = make(map[uint32]string)

var progNameMapping = make(map[uint32]string)
var progModuleMapping = make(map[uint32]string)

// AddNameMappings adds the full name mappings for ebpf maps in the manager
func AddNameMappings(mgr *manager.Manager, module string) {
	maps, err := mgr.GetMaps()
	if err != nil {
		return
	}
	iterateMaps(maps, func(mapid uint32, name string) {
		mapNameMapping[mapid] = name
		mapModuleMapping[mapid] = module
	})

	progs, err := mgr.GetPrograms()
	if err != nil {
		return
	}
	iterateProgs(progs, func(progid uint32, name string) {
		progNameMapping[progid] = name
		progModuleMapping[progid] = module
	})
}

// AddNameMappingsCollection adds the full name mappings for ebpf maps in the collection
func AddNameMappingsCollection(coll *ebpf.Collection, module string) {
	iterateMaps(coll.Maps, func(mapid uint32, name string) {
		mapNameMapping[mapid] = name
		mapModuleMapping[mapid] = module
	})
	iterateProgs(coll.Programs, func(progid uint32, name string) {
		progNameMapping[progid] = name
		progModuleMapping[progid] = module
	})
}

// RemoveNameMappings removes the full name mappings for ebpf maps in the manager
func RemoveNameMappings(mgr *manager.Manager) {
	maps, err := mgr.GetMaps()
	if err != nil {
		return
	}
	iterateMaps(maps, func(mapid uint32, name string) {
		delete(mapNameMapping, mapid)
		delete(mapModuleMapping, mapid)
	})

	progs, err := mgr.GetPrograms()
	if err != nil {
		return
	}
	iterateProgs(progs, func(progid uint32, name string) {
		delete(progNameMapping, progid)
		delete(progModuleMapping, progid)
	})
}

// RemoveNameMappingsCollection removes the full name mappings for ebpf maps in the collection
func RemoveNameMappingsCollection(coll *ebpf.Collection) {
	iterateMaps(coll.Maps, func(mapid uint32, name string) {
		delete(mapNameMapping, mapid)
		delete(mapModuleMapping, mapid)
	})
	iterateProgs(coll.Programs, func(progid uint32, name string) {
		delete(progNameMapping, progid)
		delete(progModuleMapping, progid)
	})
}

func iterateMaps(maps map[string]*ebpf.Map, mapFn func(mapid uint32, name string)) {
	for name, m := range maps {
		if info, err := m.Info(); err == nil {
			if mapid, ok := info.ID(); ok {
				mapFn(uint32(mapid), name)
			}
		}
	}
}

func iterateProgs(progs map[string]*ebpf.Program, mapFn func(progid uint32, name string)) {
	for name, p := range progs {
		if info, err := p.Info(); err == nil {
			if progid, ok := info.ID(); ok {
				mapFn(uint32(progid), name)
			}
		}
	}
}
