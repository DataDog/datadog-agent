// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package py

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/sbinet/go-python"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// #include <Python.h>
import "C"

// stickyLock is a convenient wrapper to interact with the Python GIL
// from go code when using `go-python`.
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
	gstate python.PyGILState
	locked uint32 // Flag set to 1 if the lock is locked, 0 otherwise
}

const (
	pyMemModule           = "utils.py_mem"
	pyMemSummaryFunc      = "get_mem_stats"
	pyPkgModule           = "utils.py_packages"
	pyPsutilProcPath      = "psutil.PROCFS_PATH"
	pyIntegrationListFunc = "get_datadog_wheels"
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
	return &stickyLock{
		gstate: python.PyGILState_Ensure(),
		locked: 1,
	}
}

// unlock deregisters the current thread from the interpreter, unlocks the GIL
// and detaches the goroutine from the current thread.
// Thread safe ; noop when called on an already-unlocked stickylock.
func (sl *stickyLock) unlock() {
	atomic.StoreUint32(&sl.locked, 0)
	python.PyGILState_Release(sl.gstate)
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

	if python.PyErr_Occurred() == nil { // borrowed ref, no decref needed
		return "", fmt.Errorf("the error indicator is not set on the python interpreter")
	}

	ptype, pvalue, ptraceback := python.PyErr_Fetch() // new references, have to be decref'd
	defer python.PyErr_Clear()
	defer ptype.DecRef()
	defer pvalue.DecRef()
	defer ptraceback.DecRef()

	// Make sure exception values are normalized, as per python C API docs. No error to handle here
	python.PyErr_NormalizeException(ptype, pvalue, ptraceback)

	if ptraceback != nil && ptraceback.GetCPointer() != nil {
		// There's a traceback, try to format it nicely
		traceback := python.PyImport_ImportModule("traceback")
		defer traceback.DecRef()
		formatExcFn := traceback.GetAttrString("format_exception")
		if formatExcFn != nil {
			defer formatExcFn.DecRef()
			pyFormattedExc := formatExcFn.CallFunction(ptype, pvalue, ptraceback)
			if pyFormattedExc != nil {
				defer pyFormattedExc.DecRef()

				tracebackString := ""
				// "format_exception" return a list of strings (one per line)
				for i := 0; i < python.PyList_Size(pyFormattedExc); i++ {
					tracebackString = tracebackString + python.PyString_AsString(python.PyList_GetItem(pyFormattedExc, i))
				}
				return tracebackString, nil
			}
		}

		// If we reach this point, there was an error while formatting the exception
		return "", fmt.Errorf("can't format exception")
	}

	// we sometimes do not get a traceback but an error in pvalue
	if pvalue != nil && pvalue.GetCPointer() != nil {
		strPvalue := pvalue.Str()
		if strPvalue != nil {
			defer strPvalue.DecRef()
			return python.PyString_AsString(strPvalue), nil
		}
	}

	if ptype != nil {
		strPtype := ptype.Str()
		if strPtype != nil {
			defer strPtype.DecRef()
			return python.PyString_AsString(strPtype), nil
		}
	}

	return "", fmt.Errorf("unknown error")
}

// Search in module for a class deriving from baseClass and return the first match if any.
// Notice: classes that have been derived will be ignored, i.e. this function only
// returns leaves of the hierarchy tree.
// Notice: the passed `stickyLock` must be locked.
func findSubclassOf(base, module *python.PyObject, gstate *stickyLock) (*python.PyObject, error) {
	if base == nil || module == nil {
		return nil, fmt.Errorf("both base class and module must be not nil")
	}

	// baseClass is not a Class type
	if !python.PyType_Check(base) {
		return nil, fmt.Errorf("%s is not of Class type", python.PyString_AS_STRING(base.Str()))
	}

	// module is not a Module object
	if !python.PyModule_Check(module) {
		return nil, fmt.Errorf("%s is not a Module object", python.PyString_AS_STRING(module.Str()))
	}

	dir := module.PyObject_Dir()
	defer dir.DecRef()
	var class *python.PyObject
	for i := 0; i < python.PyList_GET_SIZE(dir); i++ {
		symbolName := python.PyString_AsString(python.PyList_GetItem(dir, i))
		class = module.GetAttrString(symbolName) // new ref, don't DecRef because we return it (caller is owner)

		if class == nil {
			pyErr, err := gstate.getPythonError()

			if err != nil {
				return nil, fmt.Errorf("An error occurred while searching for the base class and couldn't be formatted: %v", err)
			}
			return nil, errors.New(pyErr)
		}

		// not a class, ignore
		if !python.PyType_Check(class) {
			class.DecRef()
			continue
		}

		// this is an unrelated class, ignore
		if class.IsSubclass(base) != 1 {
			class.DecRef()
			continue
		}

		// `class` is actually `base` itself, ignore
		if class.RichCompareBool(base, python.Py_EQ) == 1 {
			class.DecRef()
			continue
		}

		// does `class` have subclasses?
		subclasses := class.CallMethod("__subclasses__")
		if subclasses == nil {
			pyErr, err := gstate.getPythonError()

			if err != nil {
				return nil, fmt.Errorf("An error occurred while checking for subclasses and couldn't be formatted: %v", err)
			}
			return nil, errors.New(pyErr)
		}

		subclassesCount := python.PyList_GET_SIZE(subclasses)
		subclasses.DecRef()

		// `class` has subclasses but checks are supposed to have none, ignore
		if subclassesCount > 0 {
			class.DecRef()
			continue
		}

		// got it, return the check class
		return class, nil
	}

	return nil, fmt.Errorf("cannot find a subclass of %s in module %s",
		python.PyString_AS_STRING(base.Str()), python.PyString_AS_STRING(module.Str()))
}

// Get the rightmost component of a module path like foo.bar.baz
func getModuleName(modulePath string) string {
	toks := strings.Split(modulePath, ".")
	// no need to check toks length, worst case it contains only an empty string
	return toks[len(toks)-1]
}

func decAllRefs(refs []*python.PyObject) {
	for _, ref := range refs {
		ref.DecRef()
	}
}

// GetPythonInterpreterMemoryUsage collects python interpreter memory usage
func GetPythonInterpreterMemoryUsage() ([]*PythonStats, error) {
	glock := newStickyLock()
	defer glock.unlock()

	memStatModule := python.PyImport_ImportModule(pyMemModule)
	if memStatModule == nil {
		return nil, fmt.Errorf("Unable to import Python module: %s", pyMemModule)
	}
	defer memStatModule.DecRef()

	statter := memStatModule.GetAttrString(pyMemSummaryFunc)
	if statter == nil {
		pyErr, err := glock.getPythonError()

		if err != nil {
			return nil, fmt.Errorf("An error occurred while grabbing the python memory statter: %v", err)
		}
		return nil, errors.New(pyErr)
	}
	defer statter.DecRef()

	args := python.PyTuple_New(0)
	kwargs := python.PyDict_New()
	defer args.DecRef()
	defer kwargs.DecRef()

	stats := statter.Call(args, kwargs)
	if stats == nil {
		pyErr, err := glock.getPythonError()

		if err != nil {
			return nil, fmt.Errorf("An error occurred collecting python memory stats: %v", err)
		}
		return nil, errors.New(pyErr)
	}
	defer stats.DecRef()

	keys := python.PyDict_Keys(stats)
	if keys == nil {
		pyErr, err := glock.getPythonError()

		if err != nil {
			return nil, fmt.Errorf("An error occurred collecting python memory stats: %v", err)
		}
		return nil, errors.New(pyErr)
	}
	defer keys.DecRef()

	myPythonStats := []*PythonStats{}
	var entry *python.PyObject
	for i := 0; i < python.PyList_GET_SIZE(keys); i++ {
		entryName := python.PyString_AsString(python.PyList_GetItem(keys, i))
		entry = python.PyDict_GetItemString(stats, entryName)
		if entry == nil {
			pyErr, err := glock.getPythonError()

			if err != nil {
				log.Warnf("An error occurred while iterating the memory entry : %v", err)
			} else {
				log.Warnf("%v", pyErr)
			}

			continue
		}

		n := python.PyDict_GetItemString(entry, "n")
		if n == nil {
			pyErr, err := glock.getPythonError()

			if err != nil {
				log.Warnf("An error occurred while iterating the memory entry : %v", err)
			} else {
				log.Warnf("%v", pyErr)
			}

			continue
		}

		sz := python.PyDict_GetItemString(entry, "sz")
		if sz == nil {
			pyErr, err := glock.getPythonError()

			if err != nil {
				log.Warnf("An error occurred while iterating the memory entry : %v", err)
			} else {
				log.Warnf("%v", pyErr)
			}

			continue
		}

		pyStat := &PythonStats{
			Type:     entryName,
			NObjects: python.PyInt_AsLong(n),
			Size:     python.PyInt_AsLong(sz),
			Entries:  []*PythonStatsEntry{},
		}

		entries := python.PyDict_GetItemString(entry, "entries")
		if entries == nil {
			continue
		}

		for i := 0; i < python.PyList_GET_SIZE(entries); i++ {
			ref := python.PyList_GetItem(entries, i)
			if ref == nil {
				pyErr, err := glock.getPythonError()

				if err != nil {
					log.Warnf("An error occurred while iterating the entry details: %v", err)
				} else {
					log.Warnf("%v", pyErr)
				}

				continue
			}

			obj := python.PyList_GetItem(ref, 0)
			if obj == nil {
				pyErr, err := glock.getPythonError()

				if err != nil {
					log.Warnf("An error occurred while iterating the entry details : %v", err)
				} else {
					log.Warnf("%v", pyErr)
				}

				continue
			}

			nEntry := python.PyList_GetItem(ref, 1)
			if nEntry == nil {
				pyErr, err := glock.getPythonError()

				if err != nil {
					log.Warnf("An error occurred while iterating the entry details : %v", err)
				} else {
					log.Warnf("%v", pyErr)
				}

				continue
			}

			szEntry := python.PyList_GetItem(ref, 2)
			if szEntry == nil {
				pyErr, err := glock.getPythonError()

				if err != nil {
					log.Warnf("An error occurred while iterating the entry details : %v", err)
				} else {
					log.Warnf("%v", pyErr)
				}

				continue
			}

			pyEntry := &PythonStatsEntry{
				Reference: python.PyString_AsString(obj),
				NObjects:  python.PyInt_AsLong(nEntry),
				Size:      python.PyInt_AsLong(szEntry),
			}
			pyStat.Entries = append(pyStat.Entries, pyEntry)
		}

		myPythonStats = append(myPythonStats, pyStat)
	}

	return myPythonStats, nil
}

// GetPythonIntegrationList collects python datadog installed integrations list
func GetPythonIntegrationList() ([]string, error) {
	glock := newStickyLock()
	defer glock.unlock()

	pkgModule := python.PyImport_ImportModule(pyPkgModule)
	if pkgModule == nil {
		return nil, fmt.Errorf("Unable to import Python module: %s", pyPkgModule)
	}
	defer pkgModule.DecRef()

	pkgLister := pkgModule.GetAttrString(pyIntegrationListFunc)
	if pkgLister == nil {
		pyErr, err := glock.getPythonError()

		if err != nil {
			return nil, fmt.Errorf("An error occurred while grabbing the python datadog integration list: %v", err)
		}
		return nil, errors.New(pyErr)
	}
	defer pkgLister.DecRef()

	args := python.PyTuple_New(0)
	kwargs := python.PyDict_New()
	defer args.DecRef()
	defer kwargs.DecRef()

	packages := pkgLister.Call(args, kwargs)
	if packages == nil {
		pyErr, err := glock.getPythonError()

		if err != nil {
			return nil, fmt.Errorf("An error occurred compiling the list of python integrations: %v", err)
		}
		return nil, errors.New(pyErr)
	}
	defer packages.DecRef()

	ddPythonPackages := []string{}
	for i := 0; i < python.PyList_Size(packages); i++ {
		pkgName := python.PyString_AsString(python.PyList_GetItem(packages, i))
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

	ns := strings.Split(pyPsutilProcPath, ".")
	pyPsutilModule := ns[0]
	psutilModule := python.PyImport_ImportModule(pyPsutilModule)
	if psutilModule == nil {
		pyErr, err := glock.getPythonError()

		if err != nil {
			return fmt.Errorf("Error importing python psutil module: %v", err)
		}
		return errors.New(pyErr)
	}
	defer psutilModule.DecRef()

	pyProcPath := python.PyString_FromString(procPath)
	defer pyProcPath.DecRef()

	ret := psutilModule.SetAttrString(ns[1], pyProcPath)
	if ret == -1 {
		pyErr, err := glock.getPythonError()

		if err != nil {
			return fmt.Errorf("An error setting the psutil procfs path: %v", err)
		}
		return errors.New(pyErr)
	}
	return nil
}
