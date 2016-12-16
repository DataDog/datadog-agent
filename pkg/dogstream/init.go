package dogstream

import python "github.com/sbinet/go-python"

// #cgo pkg-config: python-2.7
// #cgo linux CFLAGS: -std=gnu99
// #include <Python.h>
import "C"

// Initialize wraps all the operations needed to start the Python interpreter and
// configure the environment
func Initialize(paths ...string) *python.PyThreadState {
	// Disable Site initialization
	C.Py_NoSiteFlag = 1

	// Start the interpreter
	if C.Py_IsInitialized() == 0 {
		C.Py_Initialize()
	}
	if C.Py_IsInitialized() == 0 {
		panic("python: could not initialize the python interpreter")
	}

	// make sure the GIL is correctly initialized
	if C.PyEval_ThreadsInitialized() == 0 {
		C.PyEval_InitThreads()
	}
	if C.PyEval_ThreadsInitialized() == 0 {
		panic("python: could not initialize the GIL")
	}

	// Set the PYTHONPATH if needed
	if len(paths) > 0 {
		path := python.PySys_GetObject("path")
		for _, p := range paths {
			python.PyList_Append(path, python.PyString_FromString(p))
		}
	}

	// we acquired the GIL to initialize threads but from this point
	// we don't need it anymore, let's release it
	state := python.PyEval_SaveThread()

	// return the state so the caller can resume
	return state
}
