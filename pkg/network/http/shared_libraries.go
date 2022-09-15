// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unsafe"

	psfilepath "github.com/DataDog/gopsutil/process/filepath"
	"github.com/DataDog/gopsutil/process/so"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func toLibPath(data []byte) libPath {
	return *(*libPath)(unsafe.Pointer(&data[0]))
}

func (l *libPath) Bytes() []byte {
	return l.Buf[:l.Len]
}

// syncInterval controls the frequency at which /proc/<PID>/maps are inspected.
// this is to ensure that we remove/deregister the shared libraries that are no
// longer mapped into memory.
const soSyncInterval = 5 * time.Minute

type soRule struct {
	re           *regexp.Regexp
	registerCB   func(string) error
	unregisterCB func(string) error
}

// soWatcher provides a way to tie callback functions to the lifecycle of shared libraries
type soWatcher struct {
	procRoot   string
	hostMount  string
	all        *regexp.Regexp
	rules      []soRule
	loadEvents *ddebpf.PerfHandler
	registry   *soRegistry
}

type seenKey struct {
	pid, path string
}

func newSOWatcher(procRoot string, perfHandler *ddebpf.PerfHandler, rules ...soRule) *soWatcher {
	allFilters := make([]string, len(rules))
	for i, r := range rules {
		allFilters[i] = r.re.String()
	}

	all := regexp.MustCompile(fmt.Sprintf("(%s)", strings.Join(allFilters, "|")))
	return &soWatcher{
		procRoot:   procRoot,
		hostMount:  os.Getenv("HOST_ROOT"),
		all:        all,
		rules:      rules,
		loadEvents: perfHandler,
		registry: &soRegistry{
			byPath:  make(map[string]*soRegistration),
			byInode: make(map[uint64]*soRegistration),
		},
	}
}

// Start consuming shared-library events
func (w *soWatcher) Start() {
	seen := make(map[seenKey]struct{})
	w.sync()
	go func() {
		ticker := time.NewTicker(soSyncInterval)
		defer ticker.Stop()
		thisPID, _ := util.GetRootNSPID()

		for {
			select {
			case <-ticker.C:
				seen = make(map[seenKey]struct{})
				w.sync()
			case event, ok := <-w.loadEvents.DataChannel:
				if !ok {
					return
				}

				lib := toLibPath(event.Data)
				// if this shared library was loaded by system-probe we ignore it.
				// this is to avoid a feedback-loop since the shared libraries here monitored
				// end up being opened by system-probe
				if int(lib.Pid) == thisPID {
					event.Done()
					break
				}

				path := lib.Bytes()
				for _, r := range w.rules {
					if !r.re.Match(path) {
						continue
					}

					var (
						libPath = string(path)
						pidPath = fmt.Sprintf("%s/%d", w.procRoot, lib.Pid)
					)

					// resolving paths is expensive so we cache the libraries we've already seen
					k := seenKey{pidPath, libPath}
					if _, ok := seen[k]; ok {
						break
					}
					seen[k] = struct{}{}

					// resolve namespaced path to host path
					pathResolver := psfilepath.NewResolver(w.procRoot)
					pathResolver.LoadPIDMounts(pidPath)
					if hostPath := pathResolver.Resolve(libPath); hostPath != "" {
						libPath = hostPath
					}

					libPath = w.canonicalizePath(libPath)
					w.registry.register(libPath, r)
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

func (w *soWatcher) sync() {
	libraries := so.FindProc(w.procRoot, w.all)
	toUnregister := make(map[string]struct{}, len(w.registry.byPath))
	for libPath := range w.registry.byPath {
		toUnregister[libPath] = struct{}{}
	}

	for _, lib := range libraries {
		path := w.canonicalizePath(lib.HostPath)
		if _, ok := w.registry.byPath[path]; ok {
			// shared library still mapped into memory
			// don't unregister it
			delete(toUnregister, path)
			continue
		}

		for _, r := range w.rules {
			if r.re.MatchString(path) {
				// new library detected
				// register it
				w.registry.register(path, r)
				break
			}
		}
	}

	for path := range toUnregister {
		w.registry.unregister(path)
	}
}

func (w *soWatcher) canonicalizePath(path string) string {
	if w.hostMount != "" {
		path = filepath.Join(w.hostMount, path)
	}

	return followSymlink(path)
}

type soRegistration struct {
	inode        uint64
	refcount     int
	unregisterCB func(string) error
}

type soRegistry struct {
	byPath  map[string]*soRegistration
	byInode map[uint64]*soRegistration
}

func (r *soRegistry) register(libPath string, rule soRule) {
	if _, ok := r.byPath[libPath]; ok {
		return
	}

	inode, err := getInode(libPath)
	if err != nil {
		return
	}

	if registration, ok := r.byInode[inode]; ok {
		registration.refcount++
		r.byPath[libPath] = registration
		log.Debugf("registering library=%s", libPath)
		return
	}

	err = rule.registerCB(libPath)
	if err != nil {
		log.Debugf("error registering library=%s: %s", libPath, err)
		if err := rule.unregisterCB(libPath); err != nil {
			log.Debugf("unregisterCB %s : %s", libPath, err)
		}

		// save sentinel value so we don't attempt to re-register shared
		// libraries that are problematic for some reason
		registration := newRegistration(inode, nil)
		r.byPath[libPath] = registration
		r.byInode[inode] = registration
		return
	}

	log.Debugf("registering library=%s", libPath)
	registration := newRegistration(inode, rule.unregisterCB)
	r.byPath[libPath] = registration
	r.byInode[inode] = registration
}

func (r *soRegistry) unregister(libPath string) {
	registration, ok := r.byPath[libPath]
	if !ok {
		return
	}

	log.Debugf("unregistering library=%s", libPath)
	delete(r.byPath, libPath)
	registration.refcount--
	if registration.refcount > 0 {
		return
	}

	delete(r.byInode, registration.inode)
	if registration.unregisterCB != nil {
		err := registration.unregisterCB(libPath)
		if err != nil {
			log.Debugf("unregisterCB %s : %s", libPath, err)
		}
	}
}

func newRegistration(inode uint64, unregisterCB func(string) error) *soRegistration {
	return &soRegistration{
		inode:        inode,
		unregisterCB: unregisterCB,
		refcount:     1,
	}
}

func followSymlink(path string) string {
	if withoutSymLinks, err := filepath.EvalSymlinks(path); err == nil {
		return withoutSymLinks
	}

	return path
}

func getInode(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("invalid file stat")
	}

	return stat.Ino, nil
}
