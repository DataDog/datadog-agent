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

// pathIdentifier is the unique key (system wide) based on dev/inode
type pathIdentifier struct {
	dev   uint64
	inode uint64
}

func (p *pathIdentifier) String() string {
	return fmt.Sprintf("dev/inode %d.%d/%d", unix.Major(p.dev), unix.Minor(p.dev), p.inode)
}

func (p *pathIdentifier) Key() string {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint64(b, p.dev)
	binary.LittleEndian.PutUint64(b[8:], p.inode)
	m := murmur3.Sum64(b)
	bsum := make([]byte, 8)
	binary.LittleEndian.PutUint64(bsum, m)
	return base64.StdEncoding.EncodeToString(bsum)
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
		return pi, fmt.Errorf("invalid file stat")
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
	pathID pathIdentifier
	//mTime  uint64 TODO(nplanel) reload uprobes if the file changed (dnf upgrade)

	refcount     int
	unregisterCB func(pathIdentifier) error
}

func (r *soRegistration) Unregister() (cleanup bool) {
	r.refcount--
	if r.refcount > 0 {
		return false
	}
	if r.unregisterCB != nil {
		if err := r.unregisterCB(r.pathID); err != nil {
			log.Debugf("unregisterCB %s : %s", r.pathID.String(), err)
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
	thisPID, _ := util.GetRootNSPID()

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
			log.Debugf("process %d maps parsing failed %s", pid, err)
			return nil
		}

		root := fmt.Sprintf("%s/%d/root", w.procRoot, pid)
		for _, m := range *mmaps {
			for _, r := range w.rules {
				if !r.re.MatchString(m.Path) {
					continue
				}

				w.registry.Register(root, m.Path, uint32(pid), r)
			}
		}

		return nil
	})

	err := w.processMonitor.Initialize()
	if err != nil {
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
		//TODO(nplanel) deregister uprobes() (unlikely error path)
		defer w.processMonitor.Stop()

		for {
			select {
			case event, ok := <-w.loadEvents.DataChannel:
				if !ok {
					return
				}

				lib := toLibPath(event.Data)
				var (
					path    = lib.Bytes()
					libPath = string(path)
					procPid = fmt.Sprintf("%s/%d", w.procRoot, lib.Pid)
					root    = procPid + "/root"
					cwd     = procPid + "/cwd"
				)
				if strings.HasPrefix(libPath, "/proc/") { //lib.Pid == uint32(thisPID) { // don't scan ourself when we resolve offsets
					continue
				}

				for _, r := range w.rules {
					if !r.re.Match(path) {
						continue
					}

					// use cwd of the process as root if the path is absolute
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

func (r *soRegistry) Unregister(pid uint32) {
	r.m.Lock()
	defer r.m.Unlock()

	reg, found := r.byPID[pid]
	if !found {
		return
	}
	if reg.Unregister() == true {
		// we need to cleanup our entries as there are no more process using this ELF
		delete(r.byID, reg.pathID)
	}
	delete(r.byPID, pid)
}

func (r *soRegistry) Register(root string, libPath string, pid uint32, rule soRule) {
	hostLibPath := root + libPath
	pathID, err := newPathIdentifier(hostLibPath)
	if err != nil {
		// short living process can't hit here
		// as we receive the openat() syscall info after receiving the EXIT netlink process
		log.Debugf("can't create path identifier %s", err)
		return
	}
	if _, found := r.blocklistByID[pathID]; found {
		return
	}

	r.m.Lock()
	defer r.m.Unlock()
	if reg, found := r.byID[pathID]; found {
		reg.refcount++
		r.byPID[pid] = reg
		return
	}

	if err := rule.registerCB(pathID, root, libPath); err != nil {
		log.Debugf("error registering library %s path %s by pid %d : %s", pathID.String(), hostLibPath, pid, err)
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
