// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package utils

import (
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
)

// FileRegistry keeps track of files (uniquely determined by their underlying
// PathIdentifier) and processes actively using them.
type FileRegistry struct {
	m     sync.RWMutex
	byID  map[PathIdentifier]*registration
	byPID map[uint32]pathIdentifierSet

	// if we can't register a uprobe we don't try more than once
	blocklistByID pathIdentifierSet

	telemetry registryTelemetry
}

type activationCB func(id PathIdentifier, root string, path string) error
type deactivationCB func(id PathIdentifier) error

func NewFileRegistry() *FileRegistry {
	metricGroup := telemetry.NewMetricGroup(
		"usm.so_watcher",
		telemetry.OptPayloadTelemetry,
	)

	return &FileRegistry{
		byID:          make(map[PathIdentifier]*registration),
		byPID:         make(map[uint32]pathIdentifierSet),
		blocklistByID: make(pathIdentifierSet),
		telemetry: registryTelemetry{
			libHookFailed:               metricGroup.NewCounter("hook_failed"),
			libRegistered:               metricGroup.NewCounter("registered"),
			libAlreadyRegistered:        metricGroup.NewCounter("already_registered"),
			libBlocked:                  metricGroup.NewCounter("blocked"),
			libUnregistered:             metricGroup.NewCounter("unregistered"),
			libUnregisterNoCB:           metricGroup.NewCounter("unregister_no_callback"),
			libUnregisterErrors:         metricGroup.NewCounter("unregister_errors"),
			libUnregisterFailedCB:       metricGroup.NewCounter("unregister_failed_cb"),
			libUnregisterPathIDNotFound: metricGroup.NewCounter("unregister_path_id_not_found"),
		},
	}
}

// Register a ELF library root/libPath as be used by the pid
// Only one registration will be done per ELF (system wide)
func (r *FileRegistry) Register(root, libPath string, pid uint32, activate activationCB, deactivate deactivationCB) {
	hostLibPath := root + libPath
	pathID, err := NewPathIdentifier(hostLibPath)
	if err != nil {
		// short living process can hit here
		// as we receive the openat() syscall info after receiving the EXIT netlink process
		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("can't create path identifier %s", err)
		}
		return
	}

	r.m.Lock()
	defer r.m.Unlock()
	if _, found := r.blocklistByID[pathID]; found {
		r.telemetry.libBlocked.Add(1)
		return
	}

	if reg, found := r.byID[pathID]; found {
		if _, found := r.byPID[pid][pathID]; !found {
			reg.uniqueProcessesCount.Inc()
			// Can happen if a new process opens the same so.
			if len(r.byPID[pid]) == 0 {
				r.byPID[pid] = pathIdentifierSet{}
			}
			r.byPID[pid][pathID] = struct{}{}
		}
		r.telemetry.libAlreadyRegistered.Add(1)
		return
	}

	if err := activate(pathID, root, libPath); err != nil {
		log.Debugf("error registering library (adding to blocklist) %s path %s by pid %d : %s", pathID.String(), hostLibPath, pid, err)
		// we are calling UnregisterCB here as some uprobes could be already attached, UnregisterCB cleanup those entries
		if deactivate != nil {
			if err := deactivate(pathID); err != nil {
				log.Debugf("UnregisterCB library %s path %s : %s", pathID.String(), hostLibPath, err)
			}
		}
		// save sentinel value, so we don't attempt to re-register shared
		// libraries that are problematic for some reason
		r.blocklistByID[pathID] = struct{}{}
		r.telemetry.libHookFailed.Add(1)
		return
	}

	reg := r.newRegistration(deactivate)
	r.byID[pathID] = reg
	if len(r.byPID[pid]) == 0 {
		r.byPID[pid] = pathIdentifierSet{}
	}
	r.byPID[pid][pathID] = struct{}{}
	log.Debugf("registering library %s path %s by pid %d", pathID.String(), hostLibPath, pid)
	r.telemetry.libRegistered.Add(1)
}

// Unregister a pid if exists, unregisterCB will be called if his uniqueProcessesCount == 0
func (r *FileRegistry) Unregister(pid int) {
	pidU32 := uint32(pid)
	r.m.RLock()
	_, found := r.byPID[pidU32]
	r.m.RUnlock()
	if !found {
		return
	}

	r.m.Lock()
	defer r.m.Unlock()
	paths, found := r.byPID[pidU32]
	if !found {
		return
	}
	for pathID := range paths {
		reg, found := r.byID[pathID]
		if !found {
			r.telemetry.libUnregisterPathIDNotFound.Add(1)
			continue
		}
		if reg.unregisterPath(pathID) {
			// we need to clean up our entries as there are no more processes using this ELF
			delete(r.byID, pathID)
		}
	}
	delete(r.byPID, pidU32)
}

func (r *FileRegistry) GetRegisteredProcesses() map[int32]struct{} {
	r.m.RLock()
	defer r.m.RUnlock()

	result := make(map[int32]struct{}, len(r.byPID))
	for pid := range r.byPID {
		result[int32(pid)] = struct{}{}
	}
	return result
}

// cleanup removes all registrations.
// This function should be called in the termination, and after we're stopping all other goroutines.
func (r *FileRegistry) Cleanup() {
	for pathID, reg := range r.byID {
		reg.unregisterPath(pathID)
	}
}

// This method will be removed from the the public API and it's just here to ensure
// that we're not breaking anything during the refactoring since the sharedlibraries.Watcher
// tests are currently very tied to internal state of the registry
func (r *FileRegistry) PathIDExists(pathID PathIdentifier) bool {
	r.m.RLock()
	defer r.m.RUnlock()

	_, ok := r.byID[pathID]
	return ok
}

// This method will be removed from the the public API and it's just here to ensure
// that we're not breaking anything during the refactoring since the sharedlibraries.Watcher
// tests are currently very tied to internal state of the registry
func (r *FileRegistry) IsPIDAssociatedToPathID(pid uint32, pathID PathIdentifier) bool {
	r.m.RLock()
	defer r.m.RUnlock()

	value, ok := r.byPID[pid]
	if !ok {
		return false
	}
	_, ok = value[pathID]
	return ok
}

func (r *FileRegistry) newRegistration(unregister func(PathIdentifier) error) *registration {
	uniqueCounter := atomic.Int32{}
	uniqueCounter.Store(int32(1))
	return &registration{
		unregisterCB:         unregister,
		uniqueProcessesCount: uniqueCounter,
		telemetry:            &r.telemetry,
	}
}

type registration struct {
	uniqueProcessesCount atomic.Int32
	unregisterCB         func(PathIdentifier) error

	// we are sharing the telemetry from FileRegistry
	telemetry *registryTelemetry
}

// unregister return true if there are no more reference to this registration
func (r *registration) unregisterPath(pathID PathIdentifier) bool {
	currentUniqueProcessesCount := r.uniqueProcessesCount.Dec()
	if currentUniqueProcessesCount > 0 {
		return false
	}
	if currentUniqueProcessesCount < 0 {
		log.Errorf("unregistered %+v too much (current counter %v)", pathID, currentUniqueProcessesCount)
		r.telemetry.libUnregisterErrors.Add(1)
		return true
	}

	// currentUniqueProcessesCount is 0, thus we should unregister.
	if r.unregisterCB != nil {
		if err := r.unregisterCB(pathID); err != nil {
			// Even if we fail here, we have to return true, as best effort methodology.
			// We cannot handle the failure, and thus we should continue.
			log.Errorf("error while unregistering %s : %s", pathID.String(), err)
			r.telemetry.libUnregisterFailedCB.Add(1)
		}
	} else {
		r.telemetry.libUnregisterNoCB.Add(1)
	}
	r.telemetry.libUnregistered.Add(1)
	return true
}

type registryTelemetry struct {
	// a library can be :
	//  o Registered : it's a new library
	//  o AlreadyRegistered : we have already hooked (uprobe) this library (unique by pathID)
	//  o HookFailed : uprobe registration failed for one library
	//  o Blocked : previous uprobe registration failed, so we block further call
	//  o Unregistered : a library hook is unregistered, meaning there are no more refcount to the corresponding pathID
	//  o UnregisterNoCB : unregister event has been done but the rule doesn't have an unregister callback
	//  o UnregisterErrors : we encounter an error during the unregistration, looks at the logs for further details
	//  o UnregisterFailedCB : we encounter an error during the callback unregistration, looks at the logs for further details
	//  o UnregisterPathIDNotFound : we can't find the pathID registration, it's a bug, this value should be always 0
	libRegistered               *telemetry.Counter
	libAlreadyRegistered        *telemetry.Counter
	libHookFailed               *telemetry.Counter
	libBlocked                  *telemetry.Counter
	libUnregistered             *telemetry.Counter
	libUnregisterNoCB           *telemetry.Counter
	libUnregisterErrors         *telemetry.Counter
	libUnregisterFailedCB       *telemetry.Counter
	libUnregisterPathIDNotFound *telemetry.Counter
}
