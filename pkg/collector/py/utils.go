// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package py

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/sbinet/go-python"
)

// #include <Python.h>
import "C"

// stickyLock is a convenient wrapper to interact with the Python GIL
// from go code when using `go-python`.
//
// We are going to call the Python C API from different goroutines that
// in turn will be executed on mulitple, different threads, making the
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
		formatExcFn := traceback.GetAttrString("format_exception")
		if formatExcFn != nil {
			defer formatExcFn.DecRef()
			pyFormattedExc := formatExcFn.CallFunction(ptype, pvalue, ptraceback)
			if pyFormattedExc != nil {
				defer pyFormattedExc.DecRef()
				pyStringExc := pyFormattedExc.Str()
				if pyStringExc != nil {
					defer pyStringExc.DecRef()
					return python.PyString_AsString(pyStringExc), nil
				}
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
// Notice: the passed `stickyLock` must be locked
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
		symbolName := python.PyString_AsString(python.PyList_GET_ITEM(dir, i))
		class = module.GetAttrString(symbolName) // new ref, don't DecRef because we return it (caller is owner)

		if class == nil {
			pyErr, err := gstate.getPythonError()

			if err != nil {
				return nil, fmt.Errorf("An error occurred while searching for the AgentCheck class and couldn't be formatted: %v", err)
			}
			return nil, errors.New(pyErr)
		}

		if !python.PyType_Check(class) {
			// ignore this item
			class.DecRef()
			continue
		}

		// IsSubclass returns success if class is the same, we need to go deeper
		if class.IsSubclass(base) == 1 && class.RichCompareBool(base, python.Py_EQ) != 1 {
			return class, nil
		}
	}
	return nil, fmt.Errorf("cannot find a subclass of %s in module %s",
		python.PyString_AS_STRING(base.Str()), python.PyString_AS_STRING(base.Str()))
}

// Get the rightmost component of a module path like foo.bar.baz
func getModuleName(modulePath string) string {
	toks := strings.Split(modulePath, ".")
	// no need to check toks length, worst case it contains only an empty string
	return toks[len(toks)-1]
}
