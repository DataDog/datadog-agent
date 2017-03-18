package py

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/sbinet/go-python"
)

// #include <Python.h>
import "C"

// StickyLock embeds a global state that is locked upon creation
// and makes the current goroutine be locked to the current thread
type StickyLock struct {
	gstate python.PyGILState
}

// NewStickyLock locks the GIL and sticks the goroutine to the current thread
func NewStickyLock() *StickyLock {
	runtime.LockOSThread()
	return &StickyLock{
		gstate: python.PyGILState_Ensure(),
	}
}

// Unlock unlock the GIL and detach the goroutine from the current thread
func (sl *StickyLock) Unlock() {
	python.PyGILState_Release(sl.gstate)
	runtime.UnlockOSThread()
}

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

	// inject the aggregator into global namespace
	// (it will handle the GIL by itself)
	initAPI()

	// inject the datadog_agent package into global namespace
	// (it will handle the GIL by itself)
	initDatadogAgent()

	// return the state so the caller can resume
	return state
}

// Search in module for a class deriving from baseClass and return the first match if any.
func findSubclassOf(base, module *python.PyObject) (*python.PyObject, error) {
	// Lock the GIL and release it at the end of the run
	gstate := NewStickyLock()
	defer gstate.Unlock()

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
	var class *python.PyObject
	for i := 0; i < python.PyList_GET_SIZE(dir); i++ {
		symbolName := python.PyString_AsString(python.PyList_GET_ITEM(dir, i))
		class = module.GetAttrString(symbolName)

		if !python.PyType_Check(class) {
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

// GetInterpreterVersion should go in `go-python`, TODO.
func GetInterpreterVersion() string {
	// Lock the GIL and release it at the end of the run
	gstate := NewStickyLock()
	defer gstate.Unlock()

	res := C.Py_GetVersion()
	if res == nil {
		return ""
	}

	return C.GoString(res)
}
