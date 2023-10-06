// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/cihub/seelog"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FileRegistry is responsible for tracking open files and executing callbacks
// *once* when they become "active" and *once* when they became "inactive",
// which means the point in time when no processes hold a file descriptor
// pointing to it.
//
// Internally, we essentially store a reference counter for each
// `PathIdentifier`, which can be thought of as a global identifier for a file
// (a device/inode tuple);
//
// We consider a file to be active when there is one or more open file descriptors
// pointing to it (reference count >= 1), and inactivate when all processes previously
// referencing terminate (reference count == 0);
//
// The following example demonstrates the basic functionality of the `FileRegistry`:
//
// PID 50 opens /foobar => *activation* callback is executed; /foobar references=1
// PID 60 opens /foobar => no callback is executed; /foobar references=2
// PID 50 terminates => no callback is executed; /foobar references=1
// PID 60 terminates => *deactivation* callback is executed; /foobar references=0
type FileRegistry struct {
	m        sync.RWMutex
	stopped  bool
	procRoot string
	byID     map[PathIdentifier]*registration
	byPID    map[uint32]pathIdentifierSet

	// if we can't execute a callback for a given file we don't try more than once
	blocklistByID *simplelru.LRU[PathIdentifier, struct{}]

	telemetry registryTelemetry
}

// FilePath represents the location of a file from the *root* namespace view
type FilePath struct {
	HostPath string
	ID       PathIdentifier
}

func NewFilePath(procRoot, namespacedPath string, pid uint32) (FilePath, error) {
	// Use cwd of the process as root if the namespacedPath is relative
	if namespacedPath[0] != '/' {
		namespacedPath = "/cwd" + namespacedPath
	}

	path := fmt.Sprintf("%s/%d/root%s", procRoot, pid, namespacedPath)
	pathID, err := NewPathIdentifier(path)
	if err != nil {
		return FilePath{}, err
	}

	return FilePath{HostPath: path, ID: pathID}, nil
}

type callback func(FilePath) error

func NewFileRegistry(programName string) *FileRegistry {
	blocklistByID, err := simplelru.NewLRU[PathIdentifier, struct{}](2000, nil)
	if err != nil {
		log.Warnf("running without block cache list, creation error: %s", err)
		blocklistByID = nil
	}
	r := &FileRegistry{
		procRoot:      kernel.ProcFSRoot(),
		byID:          make(map[PathIdentifier]*registration),
		byPID:         make(map[uint32]pathIdentifierSet),
		blocklistByID: blocklistByID,
		telemetry:     newRegistryTelemetry(programName),
	}

	// Add self to the debugger so we can inspect internal state of this
	// FileRegistry using our debugging endpoint
	debugger.Add(r)

	return r
}

// Register inserts or updates a new file registration within to the `FileRegistry`;
//
// If no current registration exists for the given `PathIdentifier`, we execute
// its *activation* callback. Otherwise, we increment the reference counter for
// the existing registration if and only if `pid` is new;
func (r *FileRegistry) Register(namespacedPath string, pid uint32, activationCB, deactivationCB callback) {
	if activationCB == nil || deactivationCB == nil {
		log.Errorf("activationCB and deactivationCB must be both non-nil")
		return
	}

	path, err := NewFilePath(r.procRoot, namespacedPath, pid)
	if err != nil {
		// short living process can hit here
		// as we receive the openat() syscall info after receiving the EXIT netlink process
		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("can't create path identifier %s", err)
		}
		return
	}

	pathID := path.ID
	r.m.Lock()
	defer r.m.Unlock()
	if r.stopped {
		return
	}

	if r.blocklistByID != nil {
		if _, found := r.blocklistByID.Get(pathID); found {
			r.telemetry.fileBlocked.Add(1)
			return
		}
	}

	if reg, found := r.byID[pathID]; found {
		if _, found := r.byPID[pid][pathID]; !found {
			reg.uniqueProcessesCount.Inc()
			// can happen if a new process opens an already active file
			if len(r.byPID[pid]) == 0 {
				r.byPID[pid] = pathIdentifierSet{}
			}
			r.byPID[pid][pathID] = struct{}{}
		}
		r.telemetry.fileAlreadyRegistered.Add(1)
		return
	}

	if err := activationCB(path); err != nil {
		// short living process would be hard to catch and will failed when we try to open the library
		// so let's failed silently
		if errors.Is(err, os.ErrNotExist) {
			return
		}

		// we are calling `deactivationCB` here as some uprobes could be already attached
		_ = deactivationCB(FilePath{ID: pathID})
		if r.blocklistByID != nil {
			// add `pathID` to blocklist so we don't attempt to re-register files
			// that are problematic for some reason
			r.blocklistByID.Add(pathID, struct{}{})
		}
		r.telemetry.fileHookFailed.Add(1)
		return
	}

	reg := r.newRegistration(namespacedPath, deactivationCB)
	r.byID[pathID] = reg
	if len(r.byPID[pid]) == 0 {
		r.byPID[pid] = pathIdentifierSet{}
	}
	r.byPID[pid][pathID] = struct{}{}
	r.telemetry.fileRegistered.Add(1)
	r.telemetry.totalFiles.Set(int64(len(r.byID)))
	r.telemetry.totalPIDs.Set(int64(len(r.byPID)))
	log.Debugf("registering file %s path %s by pid %d", pathID.String(), path.HostPath, pid)
	return
}

// Unregister a PID if it exists
//
// All files that were previously referenced by the given PID will have their
// reference counters decremented by one. For any file for the number of
// references drops to zero, we'll execute the *deactivationCB* previously
// supplied during the `Register` call.
func (r *FileRegistry) Unregister(pid uint32) {
	r.m.Lock()
	defer r.m.Unlock()
	if r.stopped {
		return
	}

	paths, found := r.byPID[pid]
	if !found {
		return
	}

	for pathID := range paths {
		reg, found := r.byID[pathID]
		if !found {
			r.telemetry.fileUnregisterPathIDNotFound.Add(1)
			continue
		}
		if reg.unregisterPath(pathID) {
			// we need to clean up our entries as there are no more processes using this ELF
			delete(r.byID, pathID)
		}
	}
	delete(r.byPID, pid)
	r.telemetry.totalFiles.Set(int64(len(r.byID)))
	r.telemetry.totalPIDs.Set(int64(len(r.byPID)))
}

// GetRegisteredProcesses returns a set with all PIDs currently being tracked by
// the `FileRegistry`
func (r *FileRegistry) GetRegisteredProcesses() map[uint32]struct{} {
	r.m.RLock()
	defer r.m.RUnlock()

	result := make(map[uint32]struct{}, len(r.byPID))
	for pid := range r.byPID {
		result[pid] = struct{}{}
	}
	return result
}

// Log state of `FileRegistry`
func (r *FileRegistry) Log() {
	log.Debugf("file_registry summary: program=%s %s", r.telemetry.programName, r.telemetry.metricGroup.Summary())
}

// Clear removes all registrations calling their deactivation callbacks
// This function should be called once during in termination.
func (r *FileRegistry) Clear() {
	r.m.Lock()
	defer r.m.Unlock()
	if r.stopped {
		return
	}

	for pathID, reg := range r.byID {
		reg.unregisterPath(pathID)
	}
	r.stopped = true
}

func (r *FileRegistry) newRegistration(sampleFilePath string, deactivationCB callback) *registration {
	return &registration{
		deactivationCB:       deactivationCB,
		uniqueProcessesCount: atomic.NewInt32(1),
		telemetry:            &r.telemetry,
		sampleFilePath:       sampleFilePath,
	}
}

type registration struct {
	uniqueProcessesCount *atomic.Int32
	deactivationCB       callback

	// we are sharing the telemetry from FileRegistry
	telemetry *registryTelemetry

	// Note about the motivation for this field:
	// a registration is tied to a PathIdentifier which is basically a global
	// identifier to a file (dev, inode). Multiple file paths can point to the
	// same underlying (dev, inode), so the `sampleFilePath` here happens to be
	// simply *one* of these file paths and we use this only for debugging
	// purposes.
	sampleFilePath string
}

// unregister return true if there are no more reference to this registration
func (r *registration) unregisterPath(pathID PathIdentifier) bool {
	currentUniqueProcessesCount := r.uniqueProcessesCount.Dec()
	if currentUniqueProcessesCount > 0 {
		return false
	}
	if currentUniqueProcessesCount < 0 {
		log.Errorf("unregistered %+v too much (current counter %v)", pathID, currentUniqueProcessesCount)
		r.telemetry.fileUnregisterErrors.Add(1)
		return true
	}

	// currentUniqueProcessesCount is 0, thus we should unregister.
	if err := r.deactivationCB(FilePath{ID: pathID}); err != nil {
		// Even if we fail here, we have to return true, as best effort methodology.
		// We cannot handle the failure, and thus we should continue.
		log.Errorf("error while unregistering %s : %s", pathID.String(), err)
		r.telemetry.fileUnregisterFailedCB.Add(1)
	}
	r.telemetry.fileUnregistered.Add(1)
	return true
}

type registryTelemetry struct {
	programName string
	metricGroup *telemetry.MetricGroup

	// These metrics are Gauges, so their value can go up and down
	//
	// totalFiles: represents the total number of "unique file instances"
	// (dev/inode) being tracked at a given time
	//
	// totalPIDs: represents the number of processes being traced at a given
	// moment
	totalFiles *telemetry.Gauge
	totalPIDs  *telemetry.Gauge

	// a file can be :
	//  o Registered : it's a new file
	//  o AlreadyRegistered : we have already hooked (uprobe) this file (unique by pathID)
	//  o HookFailed : uprobe registration failed for one file
	//  o Blocked : previous uprobe registration failed, so we block further call
	//  o Unregistered : a file hook is unregistered, meaning there are no more refcount to the corresponding pathID
	//  o UnregisterErrors : we encounter an error during the unregistration, looks at the logs for further details
	//  o UnregisterFailedCB : we encounter an error during the callback unregistration, looks at the logs for further details
	//  o UnregisterPathIDNotFound : we can't find the pathID registration, it's a bug, this value should be always 0
	fileRegistered               *telemetry.Counter
	fileAlreadyRegistered        *telemetry.Counter
	fileHookFailed               *telemetry.Counter
	fileBlocked                  *telemetry.Counter
	fileUnregistered             *telemetry.Counter
	fileUnregisterErrors         *telemetry.Counter
	fileUnregisterFailedCB       *telemetry.Counter
	fileUnregisterPathIDNotFound *telemetry.Counter
}

func newRegistryTelemetry(programName string) registryTelemetry {
	metricGroup := telemetry.NewMetricGroup(
		"usm.file_registry",
		fmt.Sprintf("program:%s", programName),
		telemetry.OptPrometheus,
	)

	return registryTelemetry{
		programName: programName,
		metricGroup: metricGroup,

		totalFiles: metricGroup.NewGauge("total_files"),
		totalPIDs:  metricGroup.NewGauge("total_pids"),

		// Counters
		fileHookFailed:               metricGroup.NewCounter("hook_failed"),
		fileRegistered:               metricGroup.NewCounter("registered"),
		fileAlreadyRegistered:        metricGroup.NewCounter("already_registered"),
		fileBlocked:                  metricGroup.NewCounter("blocked"),
		fileUnregistered:             metricGroup.NewCounter("unregistered"),
		fileUnregisterErrors:         metricGroup.NewCounter("unregister_errors"),
		fileUnregisterFailedCB:       metricGroup.NewCounter("unregister_failed_cb"),
		fileUnregisterPathIDNotFound: metricGroup.NewCounter("unregister_path_id_not_found"),
	}
}
