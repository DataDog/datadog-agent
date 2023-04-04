// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python
// +build python

package python

import (
	"fmt"
	"runtime"
	"unsafe"

	"go.uber.org/atomic"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <stdlib.h>

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"

char *getStringAddr(char **array, unsigned int idx);
*/
import "C"

// stickyLock is a convenient wrapper to interact with the Python GIL
// from go code when using python.
//
// We are going to call the Python C API from different goroutines that
// in turn will be executed on multiple, different threads, making the
// Agent incur in this [0] sort of problems.
//
// In addition, the Go runtime might decide to pause a goroutine in a
// thread and resume it later in a different one but we cannot allow this:
// in fact, the Python interpreter will check lock/unlock requests against
// the thread ID they are called from, raising a runtime assertion if
// they don't match. To avoid this, even if giving up on some performance,
// we ask the go runtime to be sure any goroutine using a `stickyLock`
// will be always paused and resumed on the same thread.
//
// [0]: https://docs.python.org/2/c-api/init.html#non-python-created-threads
type stickyLock struct {
	gstate C.rtloader_gilstate_t
	locked *atomic.Bool
}

// PythonStatsEntry are entries for specific object type memory usage
type PythonStatsEntry struct {
	Reference string
	NObjects  int
	Size      int
}

// PythonStats contains python memory statistics
type PythonStats struct {
	Type     string
	NObjects int
	Size     int
	Entries  []*PythonStatsEntry
}

const (
	//pyMemModule           = "utils.py_mem"
	//pyMemSummaryFunc      = "get_mem_stats"
	psutilModule   = "psutil"
	psutilProcPath = "PROCFS_PATH"
)

var (
	// implements a string set of non-integrations with an empty struct map
	nonIntegrationsWheelSet = map[string]struct{}{
		"checks_base":        {},
		"checks_dev":         {},
		"checks_test_helper": {},
		"a7":                 {},
	}
)

// newStickyLock registers the current thread with the interpreter and locks
// the GIL. It also sticks the goroutine to the current thread so that a
// subsequent call to `Unlock` will unregister the very same thread.
func newStickyLock() (*stickyLock, error) {
	runtime.LockOSThread()

	pyDestroyLock.RLock()
	defer pyDestroyLock.RUnlock()

	// Ensure that rtloader isn't destroyed while we are trying to acquire GIL
	if rtloader == nil {
		return nil, fmt.Errorf("error acquiring the GIL: rtloader is not initialized")
	}

	state := C.ensure_gil(rtloader)

	return &stickyLock{
		gstate: state,
		locked: atomic.NewBool(true),
	}, nil
}

// unlock deregisters the current thread from the interpreter, unlocks the GIL
// and detaches the goroutine from the current thread.
// Thread safe ; noop when called on an already-unlocked stickylock.
func (sl *stickyLock) unlock() {
	sl.locked.Store(false)

	pyDestroyLock.RLock()
	if rtloader != nil {
		C.release_gil(rtloader, sl.gstate)
	}
	pyDestroyLock.RUnlock()

	runtime.UnlockOSThread()
}

// cStringArrayToSlice returns a slice with the contents of the char **tags (the function will not free 'array').
func cStringArrayToSlice(array **C.char) []string {
	if array != nil {
		res := []string{}
		for i := 0; ; i++ {
			// Work around go vet raising issue about unsafe pointer
			strPtr := C.getStringAddr(array, C.uint(i))
			if strPtr == nil {
				return res
			}
			str := C.GoString(strPtr)
			res = append(res, str)
		}
	}
	return nil
}

// GetPythonIntegrationList collects python datadog installed integrations list
func GetPythonIntegrationList() ([]string, error) {
	glock, err := newStickyLock()
	if err != nil {
		return nil, err
	}

	defer glock.unlock()

	integrationsList := C.get_integration_list(rtloader)
	if integrationsList == nil {
		return nil, fmt.Errorf("Could not query integration list: %s", getRtLoaderError())
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(integrationsList))
	payload := C.GoString(integrationsList)

	ddIntegrations := []string{}
	if err := yaml.Unmarshal([]byte(payload), &ddIntegrations); err != nil {
		return nil, fmt.Errorf("Could not Unmarshal integration list payload: %s", err)
	}

	ddPythonPackages := []string{}
	for _, pkgName := range ddIntegrations {
		if _, ok := nonIntegrationsWheelSet[pkgName]; ok {
			continue
		}
		ddPythonPackages = append(ddPythonPackages, pkgName)
	}
	return ddPythonPackages, nil
}

// GetIntepreterMemoryUsage collects a python interpreter memory usage snapshot
func GetPythonInterpreterMemoryUsage() ([]*PythonStats, error) {
	glock, err := newStickyLock()
	if err != nil {
		return nil, err
	}

	defer glock.unlock()

	usage := C.get_interpreter_memory_usage(rtloader)
	if usage == nil {
		return nil, fmt.Errorf("Could not collect interpreter memory snapshot: %s", getRtLoaderError())
	}
	defer C.rtloader_free(rtloader, unsafe.Pointer(usage))
	payload := C.GoString(usage)

	log.Infof("Interpreter stats received: %v", payload)

	stats := map[interface{}]interface{}{}
	if err := yaml.Unmarshal([]byte(payload), &stats); err != nil {
		return nil, fmt.Errorf("Could not Unmarshal python interpreter memory usage payload: %s", err)
	}

	myPythonStats := []*PythonStats{}
	// Let's iterate map
	for entryName, value := range stats {
		entrySummary := value.(map[interface{}]interface{})
		num := entrySummary["num"].(int)
		size := entrySummary["sz"].(int)
		entries := entrySummary["entries"].([]interface{})

		pyStat := &PythonStats{
			Type:     entryName.(string),
			NObjects: num,
			Size:     size,
			Entries:  []*PythonStatsEntry{},
		}

		for _, entry := range entries {
			contents := entry.([]interface{})
			ref := contents[0].(string)
			refNum := contents[1].(int)
			refSz := contents[2].(int)

			// add to list
			pyEntry := &PythonStatsEntry{
				Reference: ref,
				NObjects:  refNum,
				Size:      refSz,
			}
			pyStat.Entries = append(pyStat.Entries, pyEntry)
		}

		myPythonStats = append(myPythonStats, pyStat)
	}

	return myPythonStats, nil
}

// SetPythonPsutilProcPath sets python psutil.PROCFS_PATH
func SetPythonPsutilProcPath(procPath string) error {
	glock, err := newStickyLock()
	if err != nil {
		return err
	}
	defer glock.unlock()

	module := TrackedCString(psutilModule)
	defer C._free(unsafe.Pointer(module))
	attrName := TrackedCString(psutilProcPath)
	defer C._free(unsafe.Pointer(attrName))
	attrValue := TrackedCString(procPath)
	defer C._free(unsafe.Pointer(attrValue))

	C.set_module_attr_string(rtloader, module, attrName, attrValue)
	return getRtLoaderError()
}
