// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package sharedlibrary

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"go.uber.org/atomic"
)

type soRegistry struct {
	m     sync.RWMutex
	byID  map[PathIdentifier]*soRegistration
	byPID map[uint32]pathIdentifierSet

	// if we can't register a uprobe we don't try more than once
	blocklistByID pathIdentifierSet

	telemetry soRegistryTelemetry
}

func (r *soRegistry) newRegistration(unregister func(PathIdentifier) error) *soRegistration {
	uniqueCounter := atomic.Int32{}
	uniqueCounter.Store(int32(1))
	return &soRegistration{
		unregisterCB:         unregister,
		uniqueProcessesCount: uniqueCounter,
		telemetry:            &r.telemetry,
	}
}

// cleanup removes all registrations.
// This function should be called in the termination, and after we're stopping all other goroutines.
func (r *soRegistry) cleanup() {
	for pathID, reg := range r.byID {
		reg.unregisterPath(pathID)
	}
}

// unregister a pid if exists, unregisterCB will be called if his uniqueProcessesCount == 0
func (r *soRegistry) unregister(pid int) {
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

// register a ELF library root/libPath as be used by the pid
// Only one registration will be done per ELF (system wide)
func (r *soRegistry) register(root, libPath string, pid uint32, rule Rule) {
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

	if err := rule.RegisterCB(pathID, root, libPath); err != nil {
		log.Debugf("error registering library (adding to blocklist) %s path %s by pid %d : %s", pathID.String(), hostLibPath, pid, err)
		// we are calling UnregisterCB here as some uprobes could be already attached, UnregisterCB cleanup those entries
		if rule.UnregisterCB != nil {
			if err := rule.UnregisterCB(pathID); err != nil {
				log.Debugf("UnregisterCB library %s path %s : %s", pathID.String(), hostLibPath, err)
			}
		}
		// save sentinel value, so we don't attempt to re-register shared
		// libraries that are problematic for some reason
		r.blocklistByID[pathID] = struct{}{}
		r.telemetry.libHookFailed.Add(1)
		return
	}

	reg := r.newRegistration(rule.UnregisterCB)
	r.byID[pathID] = reg
	if len(r.byPID[pid]) == 0 {
		r.byPID[pid] = pathIdentifierSet{}
	}
	r.byPID[pid][pathID] = struct{}{}
	log.Debugf("registering library %s path %s by pid %d", pathID.String(), hostLibPath, pid)
	r.telemetry.libRegistered.Add(1)
}
