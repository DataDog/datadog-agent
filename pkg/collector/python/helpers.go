// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python

package python

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"unsafe"
)

// #include <datadog_agent_six.h>
// char *getStringAddr(char **array, unsigned int idx);
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
	gstate C.six_gilstate_t
	locked uint32 // Flag set to 1 if the lock is locked, 0 otherwise
}

const (
	//pyMemModule           = "utils.py_mem"
	//pyMemSummaryFunc      = "get_mem_stats"
	psutilModule   = "psutil"
	psutilProcPath = "PROCFS_PATH"
)

var (
	// implements a string set of non-intergrations with an empty stuct map
	nonIntegrationsWheelSet = map[string]struct{}{
		"checks_base":        {},
		"checks_dev":         {},
		"checks_test_helper": {},
		"a7":                 {},
	}
)

// newStickyLock register the current thread with the interpreter and locks
// the GIL. It also sticks the goroutine to the current thread so that a
// subsequent call to `Unlock` will unregister the very same thread.
func newStickyLock() *stickyLock {
	runtime.LockOSThread()
	state := C.ensure_gil(six)
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
	C.release_gil(six, sl.gstate)
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

	if C.has_error(six) == 0 {
		return "", fmt.Errorf("no error found")
	}

	return C.GoString(C.get_error(six)), nil
}

// cStringArrayToSlice returns a slice with the contents of the char **tags.
func cStringArrayToSlice(array **C.char) []string {
	if array != nil {
		goTags := []string{}
		for i := 0; ; i++ {
			// Work around go vet raising issue about unsafe pointer
			tagPtr := C.getStringAddr(array, C.uint(i))
			if tagPtr == nil {
				return goTags
			}
			tag := C.GoString(tagPtr)
			goTags = append(goTags, tag)
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

//// GetPythonInterpreterMemoryUsage collects python interpreter memory usage
//func GetPythonInterpreterMemoryUsage() ([]*PythonStats, error) {
//	glock := newStickyLock()
//	defer glock.unlock()
//
//	memStatModule := python.PyImport_ImportModule(pyMemModule)
//	if memStatModule == nil {
//		return nil, fmt.Errorf("Unable to import Python module: %s", pyMemModule)
//	}
//	defer memStatModule.DecRef()
//
//	statter := memStatModule.GetAttrString(pyMemSummaryFunc)
//	if statter == nil {
//		pyErr, err := glock.getPythonError()
//
//		if err != nil {
//			return nil, fmt.Errorf("An error occurred while grabbing the python memory statter: %v", err)
//		}
//		return nil, errors.New(pyErr)
//	}
//	defer statter.DecRef()
//
//	args := python.PyTuple_New(0)
//	kwargs := python.PyDict_New()
//	defer args.DecRef()
//	defer kwargs.DecRef()
//
//	stats := statter.Call(args, kwargs)
//	if stats == nil {
//		pyErr, err := glock.getPythonError()
//
//		if err != nil {
//			return nil, fmt.Errorf("An error occurred collecting python memory stats: %v", err)
//		}
//		return nil, errors.New(pyErr)
//	}
//	defer stats.DecRef()
//
//	keys := python.PyDict_Keys(stats)
//	if keys == nil {
//		pyErr, err := glock.getPythonError()
//
//		if err != nil {
//			return nil, fmt.Errorf("An error occurred collecting python memory stats: %v", err)
//		}
//		return nil, errors.New(pyErr)
//	}
//	defer keys.DecRef()
//
//	myPythonStats := []*PythonStats{}
//	var entry *python.PyObject
//	for i := 0; i < python.PyList_GET_SIZE(keys); i++ {
//		entryName := python.PyString_AsString(python.PyList_GetItem(keys, i))
//		entry = python.PyDict_GetItemString(stats, entryName)
//		if entry == nil {
//			pyErr, err := glock.getPythonError()
//
//			if err != nil {
//				log.Warnf("An error occurred while iterating the memory entry : %v", err)
//			} else {
//				log.Warnf("%v", pyErr)
//			}
//
//			continue
//		}
//
//		n := python.PyDict_GetItemString(entry, "n")
//		if n == nil {
//			pyErr, err := glock.getPythonError()
//
//			if err != nil {
//				log.Warnf("An error occurred while iterating the memory entry : %v", err)
//			} else {
//				log.Warnf("%v", pyErr)
//			}
//
//			continue
//		}
//
//		sz := python.PyDict_GetItemString(entry, "sz")
//		if sz == nil {
//			pyErr, err := glock.getPythonError()
//
//			if err != nil {
//				log.Warnf("An error occurred while iterating the memory entry : %v", err)
//			} else {
//				log.Warnf("%v", pyErr)
//			}
//
//			continue
//		}
//
//		pyStat := &PythonStats{
//			Type:     entryName,
//			NObjects: python.PyInt_AsLong(n),
//			Size:     python.PyInt_AsLong(sz),
//			Entries:  []*PythonStatsEntry{},
//		}
//
//		entries := python.PyDict_GetItemString(entry, "entries")
//		if entries == nil {
//			continue
//		}
//
//		for i := 0; i < python.PyList_GET_SIZE(entries); i++ {
//			ref := python.PyList_GetItem(entries, i)
//			if ref == nil {
//				pyErr, err := glock.getPythonError()
//
//				if err != nil {
//					log.Warnf("An error occurred while iterating the entry details: %v", err)
//				} else {
//					log.Warnf("%v", pyErr)
//				}
//
//				continue
//			}
//
//			obj := python.PyList_GetItem(ref, 0)
//			if obj == nil {
//				pyErr, err := glock.getPythonError()
//
//				if err != nil {
//					log.Warnf("An error occurred while iterating the entry details : %v", err)
//				} else {
//					log.Warnf("%v", pyErr)
//				}
//
//				continue
//			}
//
//			nEntry := python.PyList_GetItem(ref, 1)
//			if nEntry == nil {
//				pyErr, err := glock.getPythonError()
//
//				if err != nil {
//					log.Warnf("An error occurred while iterating the entry details : %v", err)
//				} else {
//					log.Warnf("%v", pyErr)
//				}
//
//				continue
//			}
//
//			szEntry := python.PyList_GetItem(ref, 2)
//			if szEntry == nil {
//				pyErr, err := glock.getPythonError()
//
//				if err != nil {
//					log.Warnf("An error occurred while iterating the entry details : %v", err)
//				} else {
//					log.Warnf("%v", pyErr)
//				}
//
//				continue
//			}
//
//			pyEntry := &PythonStatsEntry{
//				Reference: python.PyString_AsString(obj),
//				NObjects:  python.PyInt_AsLong(nEntry),
//				Size:      python.PyInt_AsLong(szEntry),
//			}
//			pyStat.Entries = append(pyStat.Entries, pyEntry)
//		}
//
//		myPythonStats = append(myPythonStats, pyStat)
//	}
//
//	return myPythonStats, nil
//}

// GetPythonIntegrationList collects python datadog installed integrations list
func GetPythonIntegrationList() ([]string, error) {
	glock := newStickyLock()
	defer glock.unlock()

	integrationsList := C.get_integration_list(six)
	if integrationsList == nil {
		return nil, fmt.Errorf("Could not query integration list: %s", getSixError())
	}
	defer C.six_free(six, unsafe.Pointer(integrationsList))
	payload := C.GoString(integrationsList)

	ddIntegrations := []string{}
	if err := json.Unmarshal([]byte(payload), &ddIntegrations); err != nil {
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

// SetPythonPsutilProcPath sets python psutil.PROCFS_PATH
func SetPythonPsutilProcPath(procPath string) error {
	glock := newStickyLock()
	defer glock.unlock()

	module := C.CString(psutilModule)
	defer C.Free(module)
	attrName := C.CString(psutilProcPath)
	defer C.Free(attrName)
	attrValue := C.CString(procPath)
	defer C.Free(attrValue)

	C.set_module_attr_string(six, module, attrName, attrValue)
	return getSixError()
}
