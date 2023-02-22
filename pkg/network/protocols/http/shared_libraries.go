// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/DataDog/gopsutil/process"
	"github.com/twmb/murmur3"
	"golang.org/x/sys/unix"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func toLibPath(data []byte) libPath {
	return *(*libPath)(unsafe.Pointer(&data[0]))
}

func (l *libPath) Bytes() []byte {
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
	procRoot       string
	all            *regexp.Regexp
	rules          []soRule
	loadEvents     *ddebpf.PerfHandler
	processMonitor *monitor.ProcessMonitor
	registry       *soRegistry
}

type soRegistry struct {
	m     sync.Mutex
	byID  map[pathIdentifier]*soRegistration
	byPID map[uint32]*soRegistration

	// if we can't register a uprobe we don't try more than once
	blocklistByID map[pathIdentifier]struct{}
}

func newSOWatcher(perfHandler *ddebpf.PerfHandler, rules ...soRule) *soWatcher {
	allFilters := make([]string, len(rules))
	for i, r := range rules {
		allFilters[i] = r.re.String()
	}

	all := regexp.MustCompile(fmt.Sprintf("(%s)", strings.Join(allFilters, "|")))
	return &soWatcher{
		procRoot:       util.GetProcRoot(),
		all:            all,
		rules:          rules,
		loadEvents:     perfHandler,
		processMonitor: monitor.GetProcessMonitor(),
		registry: &soRegistry{
			byID:          make(map[pathIdentifier]*soRegistration),
			byPID:         make(map[uint32]*soRegistration),
			blocklistByID: make(map[pathIdentifier]struct{}),
		},
	}
}

type soRegistration struct {
	pathID       pathIdentifier
	refcount     int
	unregisterCB func(pathIdentifier) error
}

// Unregister return true if there are no more reference to this registration
func (r *soRegistration) Unregister() bool {
	r.refcount--
	if r.refcount > 0 {
		return false
	}
	if r.unregisterCB != nil {
		if err := r.unregisterCB(r.pathID); err != nil {
			log.Debugf("error while unregisterin %s : %s", r.pathID.String(), err)
		}
	}
	return true
}

func newRegistration(pathID pathIdentifier, unregister func(pathIdentifier) error) *soRegistration {
	return &soRegistration{
		pathID:       pathID,
		unregisterCB: unregister,
		refcount:     1,
	}
}

func (w *soWatcher) processExit(pid uint32) {
	w.registry.Unregister(pid)
}

// Start consuming shared-library events
func (w *soWatcher) Start() {
	thisPID, err := util.GetRootNSPID()
	if err != nil {
		log.Warnf("soWatcher Start can't get root namespace pid %s", err)
	}

	_ = util.WithAllProcs(w.procRoot, func(pid int) error {
		if pid == thisPID { // don't uprobes ourself
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
			log.Tracef("process %d maps parsing failed %s", pid, err)
			return nil
		}

		root := fmt.Sprintf("%s/%d/root", w.procRoot, pid)
		for _, m := range *mmaps {
			for _, r := range w.rules {
				if r.re.MatchString(m.Path) {
					w.registry.Register(root, m.Path, uint32(pid), r)
					break
				}
			}
		}

		return nil
	})

	if err := w.processMonitor.Initialize(); err != nil {
		log.Errorf("can't initialize process monitor %s", err)
		return
	}
	cleanupExit, err := w.processMonitor.Subscribe(&monitor.ProcessCallback{
		Event:    monitor.EXIT,
		Metadata: monitor.ANY,
		Callback: w.processExit,
	})
	if err != nil {
		log.Errorf("can't subscribe to process monitor exit event %s", err)
		return
	}

	go func() {
		defer cleanupExit()
		defer w.processMonitor.Stop()
		// cleanup all the uprobes
		defer w.registry.cleanup()

		for {
			select {
			case event, ok := <-w.loadEvents.DataChannel:
				if !ok {
					return
				}

				lib := toLibPath(event.Data)
				path := lib.Bytes()
				libPath := string(path)
				procPid := fmt.Sprintf("%s/%d", w.procRoot, lib.Pid)
				root := procPid + "/root"
				cwd := procPid + "/cwd"

				if strings.HasPrefix(libPath, w.procRoot) {
					// don't scan ourself when we resolve offsets via debug.elf.Open()
					// but make the unit test pass as the pid of the test and the tracer would be the same
					// that why we don't filter by lib.Pid == uint32(thisPID) here
					continue
				}

				for _, r := range w.rules {
					if !r.re.Match(path) {
						continue
					}

					// use cwd of the process as root if the path is relative
					if libPath[0] != '/' {
						root = cwd
						libPath = "/" + libPath
					}

					w.registry.Register(root, libPath, lib.Pid, r)
					break
				}

				event.Done()

			case <-w.loadEvents.LostChannel:
				// Nothing to do in this case
				break
			}
		}
	}()
}

// call all the unregisterCB
func (r *soRegistry) cleanup() {
	r.m.Lock()
	defer r.m.Unlock()

	for _, reg := range r.byID {
		reg.Unregister()
	}
}

// Unregister a pid if exist, unregisterCB will be called if his refcount == 0
func (r *soRegistry) Unregister(pid uint32) {
	r.m.Lock()
	defer r.m.Unlock()

	reg, found := r.byPID[pid]
	if !found {
		return
	}
	if reg.Unregister() == true {
		// we need to cleanup our entries as there are no more processes using this ELF
		delete(r.byID, reg.pathID)
	}
	delete(r.byPID, pid)
}

// Register a ELF library root/libPath as be used by the pid
// Only one registration will be done per ELF (system wide)
func (r *soRegistry) Register(root string, libPath string, pid uint32, rule soRule) {
	hostLibPath := root + libPath
	pathID, err := newPathIdentifier(hostLibPath)
	if err != nil {
		// short living process can hit here
		// as we receive the openat() syscall info after receiving the EXIT netlink process
		log.Tracef("can't create path identifier %s", err)
		return
	}

	r.m.Lock()
	defer r.m.Unlock()
	if _, found := r.blocklistByID[pathID]; found {
		return
	}

	if reg, found := r.byID[pathID]; found {
		reg.refcount++
		r.byPID[pid] = reg
		return
	}

	if err := rule.registerCB(pathID, root, libPath); err != nil {
		log.Debugf("error registering library (adding to blocklist) %s path %s by pid %d : %s", pathID.String(), hostLibPath, pid, err)
		// we calling unregisterCB here as some uprobe could be already attached, unregisterCB will cleanup those entries
		if rule.unregisterCB != nil {
			if err := rule.unregisterCB(pathID); err != nil {
				log.Debugf("unregisterCB library %s path %s : %s", pathID.String(), hostLibPath, err)
			}
		}
		// save sentinel value so we don't attempt to re-register shared
		// libraries that are problematic for some reason
		r.blocklistByID[pathID] = struct{}{}
		return
	}

	reg := newRegistration(pathID, rule.unregisterCB)
	r.byID[pathID] = reg
	r.byPID[pid] = reg

	log.Debugf("registering library %s path %s by pid %d", pathID.String(), hostLibPath, pid)
}
