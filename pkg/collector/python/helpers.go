// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python

package python

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	yaml "gopkg.in/yaml.v2"
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
	locked uint32 // Flag set to 1 if the lock is locked, 0 otherwise
}

//PythonStatsEntry are entries for specific object type memory usage
type PythonStatsEntry struct {
	Reference string
	NObjects  int
	Size      int
}

//PythonStats contains python memory statistics
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
func newStickyLock() *stickyLock {
	runtime.LockOSThread()
	state := C.ensure_gil(rtloader)
	return &stickyLock{
		gstate: state,
		locked: 1,
	}
}

// unlock deregisters the current thread from the interpreter, unlocks the GIL
// and detaches the goroutine from the current thread.
// Thread safe ; noop when called on an already-unlocked stickylock.
func (sl *stickyLock) unlock() {
	atomic.StoreUint32(&sl.locked, 0)
	C.release_gil(rtloader, sl.gstate)
	runtime.UnlockOSThread()
}

// getPythonError returns string-formatted info about a Python interpreter error
// that occurred and clears the error flag in the Python interpreter.
//
// For many C python functions, a `NULL` return value indicates an error (always
// refer to the python C API docs to check the meaning of return values).
// If an error did occur, use this function to handle it properly.
//
// WARNING: make sure the same stickyLock was already locked when the error flag
// was set on the python interpreter
func (sl *stickyLock) getPythonError() (string, error) {
	if atomic.LoadUint32(&sl.locked) != 1 {
		return "", fmt.Errorf("the stickyLock is unlocked, can't interact with python interpreter")
	}

	if C.has_error(rtloader) == 0 {
		return "", fmt.Errorf("no error found")
	}

	return C.GoString(C.get_error(rtloader)), nil
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

// Get the rightmost component of a module path like foo.bar.baz
func getModuleName(modulePath string) string {
	toks := strings.Split(modulePath, ".")
	// no need to check toks length, worst case it contains only an empty string
	return toks[len(toks)-1]
}

// GetPythonIntegrationList collects python datadog installed integrations list
func GetPythonIntegrationList() ([]string, error) {
	if rtloader == nil {
		return nil, fmt.Errorf("rtloader is not initialized")
	}

	glock := newStickyLock()
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
	if rtloader == nil {
		return nil, fmt.Errorf("rtloader is not initialized")
	}

	glock := newStickyLock()
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
	if rtloader == nil {
		return fmt.Errorf("rtloader is not initialized")
	}

	glock := newStickyLock()
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
