// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package ebpf

import (
	"errors"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

var mappingLock sync.RWMutex

var mapNameMapping = make(map[uint32]string)
var mapModuleMapping = make(map[uint32]string)

var progNameMapping = make(map[uint32]string)
var progModuleMapping = make(map[uint32]string)

// errNoMapping is returned when a give map or program id is
// not tracked as part of system-probe/security-agent
var errNoMapping = errors.New("no mapping found for given id")

// AddProgramNameMapping manually adds a program name mapping
func AddProgramNameMapping(progid uint32, name string, module string) {
	mappingLock.Lock()
	defer mappingLock.Unlock()

	progNameMapping[progid] = name
	progModuleMapping[progid] = module
}

// AddNameMappings adds the full name mappings for ebpf maps in the manager
func AddNameMappings(mgr *manager.Manager, module string) {
	maps, err := mgr.GetMaps()
	if err != nil {
		return
	}

	mappingLock.Lock()
	defer mappingLock.Unlock()

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
	mappingLock.Lock()
	defer mappingLock.Unlock()

	iterateMaps(coll.Maps, func(mapid uint32, name string) {
		mapNameMapping[mapid] = name
		mapModuleMapping[mapid] = module
	})
	iterateProgs(coll.Programs, func(progid uint32, name string) {
		progNameMapping[progid] = name
		progModuleMapping[progid] = module
	})
}

func getMappingFromID(id uint32, m map[uint32]string) (string, error) {
	mappingLock.RLock()
	defer mappingLock.RUnlock()

	name, ok := m[id]
	if !ok {
		return "", errNoMapping
	}

	return name, nil
}

// GetMapNameFromMapID returns the map name for the given id
func GetMapNameFromMapID(id uint32) (string, error) {
	return getMappingFromID(id, mapNameMapping)
}

// GetModuleFromMapID returns the module name for the map with the given id
func GetModuleFromMapID(id uint32) (string, error) {
	return getMappingFromID(id, mapModuleMapping)

}

// GetProgNameFromProgID returns the program name for the given id
func GetProgNameFromProgID(id uint32) (string, error) {
	return getMappingFromID(id, progNameMapping)
}

// GetModuleFromProgID returns the module name for the program with the given id
func GetModuleFromProgID(id uint32) (string, error) {
	return getMappingFromID(id, progModuleMapping)
}

// RemoveNameMappings removes the full name mappings for ebpf maps in the manager
func RemoveNameMappings(mgr *manager.Manager) {
	maps, err := mgr.GetMaps()
	if err != nil {
		return
	}

	mappingLock.Lock()
	defer mappingLock.Unlock()

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

	for _, p := range mgr.Probes {
		// ebpf-manager functions like DetachHook can modify backing array of our copy of Probes out from under us
		if p == nil {
			continue
		}
		progid := p.ID()
		delete(progNameMapping, progid)
		delete(progModuleMapping, progid)
	}
}

// RemoveNameMappingsCollection removes the full name mappings for ebpf maps in the collection
func RemoveNameMappingsCollection(coll *ebpf.Collection) {
	mappingLock.Lock()
	defer mappingLock.Unlock()

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
