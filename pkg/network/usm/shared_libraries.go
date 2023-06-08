// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"go.uber.org/atomic"

	"github.com/DataDog/gopsutil/process"
	"github.com/cihub/seelog"
	"github.com/twmb/murmur3"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// The interval of the periodic scan for terminated processes. Increasing the interval, might cause larger spikes in cpu
	// and lowering it might cause constant cpu usage.
	scanTerminatedProcessesInterval = 30 * time.Second
)

func toLibPath(data []byte) http.LibPath {
	return *(*http.LibPath)(unsafe.Pointer(&data[0]))
}

func toBytes(l *http.LibPath) []byte {
	return l.Buf[:l.Len]
}

// pathIdentifier is the unique key (system wide) of a file based on dev/inode
type pathIdentifier struct {
	dev   uint64
	inode uint64
}

func (p *pathIdentifier) String() string {
	return fmt.Sprintf("dev/inode %d.%d/%d", unix.Major(p.dev), unix.Minor(p.dev), p.inode)
}

// Key is a unique (system wide) TLDR Base64(murmur3.Sum64(device, inode))
// It composes based the device (minor, major) and inode of a file
// murmur is a non-crypto hashing
//
//	As multiple containers overlayfs (same inode but could be overwritten with different binary)
//	device would be different
//
// a Base64 string representation is returned and could be used in a file path
func (p *pathIdentifier) Key() string {
	buffer := make([]byte, 16)
	binary.LittleEndian.PutUint64(buffer, p.dev)
	binary.LittleEndian.PutUint64(buffer[8:], p.inode)
	m := murmur3.Sum64(buffer)
	bufferSum := make([]byte, 8)
	binary.LittleEndian.PutUint64(bufferSum, m)
	return base64.StdEncoding.EncodeToString(bufferSum)
}

// path must be an absolute path
func newPathIdentifier(path string) (pi pathIdentifier, err error) {
	if len(path) < 1 || path[0] != '/' {
		return pi, fmt.Errorf("invalid path %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return pi, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return pi, fmt.Errorf("invalid file %q stat %T", path, info.Sys())
	}

	return pathIdentifier{
		dev:   stat.Dev,
		inode: stat.Ino,
	}, nil
}

type soRule struct {
	re           *regexp.Regexp
	registerCB   func(id pathIdentifier, root string, path string) error
	unregisterCB func(id pathIdentifier) error
}

// soWatcher provides a way to tie callback functions to the lifecycle of shared libraries
type soWatcher struct {
	wg             sync.WaitGroup
	done           chan struct{}
	procRoot       string
	rules          []soRule
	loadEvents     *ddebpf.PerfHandler
	processMonitor *monitor.ProcessMonitor
	registry       *soRegistry
}

type pathIdentifierSet = map[pathIdentifier]struct{}

type soRegistry struct {
	m     sync.RWMutex
	byID  map[pathIdentifier]*soRegistration
	byPID map[uint32]pathIdentifierSet

	// if we can't register a uprobe we don't try more than once
	blocklistByID pathIdentifierSet
}

func newSOWatcher(perfHandler *ddebpf.PerfHandler, rules ...soRule) *soWatcher {
	return &soWatcher{
		wg:             sync.WaitGroup{},
		done:           make(chan struct{}),
		procRoot:       util.GetProcRoot(),
		rules:          rules,
		loadEvents:     perfHandler,
		processMonitor: monitor.GetProcessMonitor(),
		registry: &soRegistry{
			byID:          make(map[pathIdentifier]*soRegistration),
			byPID:         make(map[uint32]pathIdentifierSet),
			blocklistByID: make(pathIdentifierSet),
		},
	}
}

type soRegistration struct {
	uniqueProcessesCount atomic.Int32
	unregisterCB         func(pathIdentifier) error
}

// unregister return true if there are no more reference to this registration
func (r *soRegistration) unregisterPath(pathID pathIdentifier) bool {
	currentUniqueProcessesCount := r.uniqueProcessesCount.Dec()
	if currentUniqueProcessesCount > 0 {
		return false
	}
	if currentUniqueProcessesCount < 0 {
		log.Errorf("unregistered %+v too much (current counter %v)", pathID, currentUniqueProcessesCount)
		return true
	}
	// currentUniqueProcessesCount is 0, thus we should unregister.
	if r.unregisterCB != nil {
		if err := r.unregisterCB(pathID); err != nil {
			// Even if we fail here, we have to return true, as best effort methodology.
			// We cannot handle the failure, and thus we should continue.
			log.Warnf("error while unregistering %s : %s", pathID.String(), err)
		}
	}
	return true
}

func newRegistration(unregister func(pathIdentifier) error) *soRegistration {
	uniqueCounter := atomic.Int32{}
	uniqueCounter.Store(int32(1))
	return &soRegistration{
		unregisterCB:         unregister,
		uniqueProcessesCount: uniqueCounter,
	}
}

func (w *soWatcher) Stop() {
	close(w.done)
	w.wg.Wait()
}

// Start consuming shared-library events
func (w *soWatcher) Start() {
	thisPID, err := util.GetRootNSPID()
	if err != nil {
		log.Warnf("soWatcher Start can't get root namespace pid %s", err)
	}

	_ = util.WithAllProcs(w.procRoot, func(pid int) error {
		if pid == thisPID { // don't scan ourself
			return nil
		}

		// report silently parsing /proc error as this could happen
		// just exit processes
		proc, err := process.NewProcess(int32(pid))
		if err != nil {
			log.Debugf("process %d parsing failed %s", pid, err)
			return nil
		}
		mmaps, err := proc.MemoryMaps(true)
		if err != nil {
			if log.ShouldLog(seelog.TraceLvl) {
				log.Tracef("process %d maps parsing failed %s", pid, err)
			}
			return nil
		}

		root := fmt.Sprintf("%s/%d/root", w.procRoot, pid)
		for _, m := range *mmaps {
			for _, r := range w.rules {
				if r.re.MatchString(m.Path) {
					w.registry.register(root, m.Path, uint32(pid), r)
					break
				}
			}
		}

		return nil
	})

	cleanupExit, err := w.processMonitor.SubscribeExit(w.registry.unregister)
	if err != nil {
		log.Errorf("can't subscribe to process monitor exit event %s", err)
		return
	}

	w.wg.Add(1)
	go func() {
		processSync := time.NewTicker(scanTerminatedProcessesInterval)

		defer func() {
			processSync.Stop()
			// Removing the registration of our hook.
			cleanupExit()
			// Stopping the process monitor (if we're the last instance)
			w.processMonitor.Stop()
			// Cleaning up all active hooks.
			w.registry.cleanup()
			// marking we're finished.
			w.wg.Done()
		}()

		for {
			select {
			case <-w.done:
				return
			case <-processSync.C:
				processSet := make(map[int32]struct{})
				w.registry.m.RLock()
				for pid := range w.registry.byPID {
					processSet[int32(pid)] = struct{}{}
				}
				w.registry.m.RUnlock()

				deletedPids := monitor.FindDeletedProcesses(processSet)
				for deletedPid := range deletedPids {
					w.registry.unregister(int(deletedPid))
				}
			case event, ok := <-w.loadEvents.DataChannel:
				if !ok {
					return
				}

				lib := toLibPath(event.Data)
				if int(lib.Pid) == thisPID {
					// don't scan ourself
					event.Done()
					continue
				}

				path := toBytes(&lib)
				libPath := string(path)
				procPid := fmt.Sprintf("%s/%d", w.procRoot, lib.Pid)
				root := procPid + "/root"
				// use cwd of the process as root if the path is relative
				if libPath[0] != '/' {
					root = procPid + "/cwd"
					libPath = "/" + libPath
				}

				for _, r := range w.rules {
					if r.re.Match(path) {
						w.registry.register(root, libPath, lib.Pid, r)
						break
					}
				}
				event.Done()
			case <-w.loadEvents.LostChannel:
				// Nothing to do in this case
				break
			}
		}
	}()
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
func (r *soRegistry) register(root, libPath string, pid uint32, rule soRule) {
	hostLibPath := root + libPath
	pathID, err := newPathIdentifier(hostLibPath)
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
		return
	}

	if err := rule.registerCB(pathID, root, libPath); err != nil {
		log.Debugf("error registering library (adding to blocklist) %s path %s by pid %d : %s", pathID.String(), hostLibPath, pid, err)
		// we are calling unregisterCB here as some uprobes could be already attached, unregisterCB cleanup those entries
		if rule.unregisterCB != nil {
			if err := rule.unregisterCB(pathID); err != nil {
				log.Debugf("unregisterCB library %s path %s : %s", pathID.String(), hostLibPath, err)
			}
		}
		// save sentinel value, so we don't attempt to re-register shared
		// libraries that are problematic for some reason
		r.blocklistByID[pathID] = struct{}{}
		return
	}

	reg := newRegistration(rule.unregisterCB)
	r.byID[pathID] = reg
	if len(r.byPID[pid]) == 0 {
		r.byPID[pid] = pathIdentifierSet{}
	}
	r.byPID[pid][pathID] = struct{}{}
	log.Debugf("registering library %s path %s by pid %d", pathID.String(), hostLibPath, pid)
}
