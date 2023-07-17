// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package usm

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"go.uber.org/atomic"

	"github.com/cihub/seelog"
	"github.com/twmb/murmur3"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
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

func newSOWatcher(perfHandler *ddebpf.PerfHandler, rules ...soRule) *soWatcher {
	metricGroup := telemetry.NewMetricGroup(
		"usm.so_watcher",
		telemetry.OptPayloadTelemetry,
	)
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

			telemetry: soRegistryTelemetry{
				libHookFailed:               metricGroup.NewCounter("hook_failed"),
				libRegistered:               metricGroup.NewCounter("registered"),
				libAlreadyRegistered:        metricGroup.NewCounter("already_registered"),
				libBlocked:                  metricGroup.NewCounter("blocked"),
				libUnregistered:             metricGroup.NewCounter("unregistered"),
				libUnregisterNoCB:           metricGroup.NewCounter("unregister_no_callback"),
				libUnregisterErrors:         metricGroup.NewCounter("unregister_errors"),
				libUnregisterFailedCB:       metricGroup.NewCounter("unregister_failed_cb"),
				libUnregisterPathIDNotFound: metricGroup.NewCounter("unregister_path_id_not_found"),
				libHits:                     metricGroup.NewCounter("hits"),
				libMatches:                  metricGroup.NewCounter("matches"),
			},
		},
	}
}

type pathIdentifierSet = map[pathIdentifier]struct{}

type soRegistryTelemetry struct {
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

	// numbers of library events from the kernel filter (Hits) and matching (Matches) the registered rules
	libHits    *telemetry.Counter
	libMatches *telemetry.Counter
}

type soRegistry struct {
	m     sync.RWMutex
	byID  map[pathIdentifier]*soRegistration
	byPID map[uint32]pathIdentifierSet

	// if we can't register a uprobe we don't try more than once
	blocklistByID pathIdentifierSet

	telemetry soRegistryTelemetry
}

type soRegistration struct {
	uniqueProcessesCount atomic.Int32
	unregisterCB         func(pathIdentifier) error

	// we are sharing the telemetry from soRegistry
	telemetry *soRegistryTelemetry
}

func (r *soRegistry) newRegistration(unregister func(pathIdentifier) error) *soRegistration {
	uniqueCounter := atomic.Int32{}
	uniqueCounter.Store(int32(1))
	return &soRegistration{
		unregisterCB:         unregister,
		uniqueProcessesCount: uniqueCounter,
		telemetry:            &r.telemetry,
	}
}

// unregister return true if there are no more reference to this registration
func (r *soRegistration) unregisterPath(pathID pathIdentifier) bool {
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

func (w *soWatcher) Stop() {
	close(w.done)
	w.wg.Wait()
}

type parseMapsFileCB func(path string)

// parseMapsFile takes in a bufio.Scanner representing a memory mapping of /proc/<PID>/maps file, and a callback to be
// applied on the paths extracted from the file. We're extracting only actual paths on the file system, and ignoring
// anonymous memory regions.
//
// Example for entries in the `maps` file:
// 7f135146b000-7f135147a000 r--p 00000000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f135147a000-7f1351521000 r-xp 0000f000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f1351521000-7f13515b8000 r--p 000b6000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
// 7f13515b8000-7f13515b9000 r--p 0014c000 fd:00 268743 /usr/lib/x86_64-linux-gnu/libm-2.31.so
func parseMapsFile(scanner *bufio.Scanner, callback parseMapsFileCB) {
	// The maps file can have multiple entries of the same loaded file, the cache is meant to ensure, we're not wasting
	// time and memory on "duplicated" hooking.
	cache := make(map[string]struct{})
	for scanner.Scan() {
		line := scanner.Text()
		cols := strings.Fields(line)
		// ensuring we have exactly 6 elements (skip '(deleted)' entries) in the line, and the 4th element (inode) is
		// not zero (indicates it is a path, and not an anonymous path).
		if len(cols) == 6 && cols[4] != "0" {
			// Check if we've seen the same path before, if so, continue to the next.
			if _, exists := cache[cols[5]]; exists {
				continue
			}
			// We didn't process the path, so cache it to avoid future re-processing.
			cache[cols[5]] = struct{}{}

			// Apply the given callback on the path.
			callback(cols[5])
		}
	}
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

		mapsPath := fmt.Sprintf("%s/%d/maps", w.procRoot, pid)
		maps, err := os.Open(mapsPath)
		if err != nil {
			log.Debugf("process %d parsing failed %s", pid, err)
			return nil
		}
		defer maps.Close()

		// Creating a callback to be applied on the paths extracted from the `maps` file.
		// We're creating the callback here, as we need the pid (which varies between iterations).
		parseMapsFileCallback := func(path string) {
			root := fmt.Sprintf("%s/%d/root", w.procRoot, pid)
			// Iterate over the rule, and look for a match.
			for _, r := range w.rules {
				if r.re.MatchString(path) {
					w.registry.register(root, path, uint32(pid), r)
					break
				}
			}
		}
		scanner := bufio.NewScanner(bufio.NewReader(maps))
		parseMapsFile(scanner, parseMapsFileCallback)
		return nil
	})

	cleanupExit := w.processMonitor.SubscribeExit(w.registry.unregister)

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

				w.registry.telemetry.libHits.Add(1)
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
						w.registry.telemetry.libMatches.Add(1)
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
		r.telemetry.libHookFailed.Add(1)
		return
	}

	reg := r.newRegistration(rule.unregisterCB)
	r.byID[pathID] = reg
	if len(r.byPID[pid]) == 0 {
		r.byPID[pid] = pathIdentifierSet{}
	}
	r.byPID[pid][pathID] = struct{}{}
	log.Debugf("registering library %s path %s by pid %d", pathID.String(), hostLibPath, pid)
	r.telemetry.libRegistered.Add(1)
}
