// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package py

import (
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	log "github.com/cihub/seelog"
	python "github.com/sbinet/go-python"
)

// #include <Python.h>
import "C"

var (
	// PythonVersion contains the interpreter version string provided by
	// `sys.version`. It's empty if the interpreter was not initialized.
	PythonVersion = ""
	// The pythonHome variable typically comes from -ldflags
	// it's needed in case the agent was built using embedded libs
	pythonHome = ""
	// see https://docs.python.org/2/c-api/init.html#c.Py_SetPythonHome
	// we should keep around the char* string we use to set the Python
	// Home through cgo until the program exits.
	pPythonHome *C.char
	// PythonHome contains the computed value of the Python Home path once the
	// intepreter is created. It might be empty in case the interpreter wasn't
	// initialized, or the Agent was built using system libs and the env var
	// PYTHONHOME is empty. It's expected to always contain a value when the
	// Agent is built using embedded libs.
	PythonHome = ""
	// PythonPath contains the string representation of the Python list returned
	// by `sys.path`. It's empty if the interpreter was not initialized.
	PythonPath = ""
)

// Initialize wraps all the operations needed to start the Python interpreter and
// configure the environment. This function should be called at most once in the
// Agent lifetime.
func Initialize(paths ...string) *python.PyThreadState {
	// Set the Python Home from within the agent if needed
	if pythonHome != "" {
		pPythonHome := C.CString(pythonHome)
		C.Py_SetPythonHome(pPythonHome)
	}

	// store the final value of Python Home in the cache
	PythonHome = C.GoString(C.Py_GetPythonHome())

	// Start the interpreter
	if C.Py_IsInitialized() == 0 {
		C.Py_Initialize()
	}
	if C.Py_IsInitialized() == 0 {
		log.Error("python: could not initialize the python interpreter")
		signals.ErrorStopper <- true
	}

	// make sure the Python threading facilities are correctly initialized,
	// please notice this will also lock the GIL, see [0] for reference.
	//
	// [0]: https://docs.python.org/2/c-api/init.html#c.PyEval_InitThreads
	if C.PyEval_ThreadsInitialized() == 0 {
		C.PyEval_InitThreads()
	}
	if C.PyEval_ThreadsInitialized() == 0 {
		log.Error("python: could not initialize the GIL")
		signals.ErrorStopper <- true
	}

	// Set the PYTHONPATH if needed.
	// We still hold a lock from calling `C.PyEval_InitThreads()` above, so we can
	// safely use go-python here without any additional loking operation.
	if len(paths) > 0 {
		path := python.PySys_GetObject("path") // borrowed ref
		for _, p := range paths {
			newPath := python.PyString_FromString(p)
			defer newPath.DecRef()
			python.PyList_Append(path, newPath)
		}
	}

	// store the Python version after killing \n chars within the string
	if res := C.Py_GetVersion(); res != nil {
		PythonVersion = strings.Replace(C.GoString(res), "\n", "", -1)
	}

	// store the Python path
	if pyList := python.PySys_GetObject("path"); pyList != nil {
		PythonPath = python.PyString_AS_STRING(pyList.Str())
	}

	// We acquired the GIL as a side effect of threading initialization (see above)
	// but from this point on we don't need it anymore. Let's reset the current thread
	// state and release the GIL, meaning that from now on any piece of code needing
	// Python needs to take care of thread state and the GIL on its own.
	// The previous thread state is returned to the caller so it can be stored and
	// reused when needed (e.g. to finalize the interpreter on exit).
	state := python.PyEval_SaveThread()

	// inject synthetic modules into the global namespace of the embedded interpreter
	// (all these calls will take care of the GIL)
	initAPI()          // `aggregator` module
	initDatadogAgent() // `datadog_agent` module

	// return the state so the caller can resume
	return state
}
