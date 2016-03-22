package py

// #cgo pkg-config: python-2.7
// #include "Python.h"
// #include <stdlib.h>
// #include <string.h>
// #include <pthread.h>
// #include <signal.h>
import "C"

import (
	"fmt"
	"runtime"
	"time"

	// 3rd party

	"github.com/op/go-logging"
	"github.com/sbinet/go-python"
)

func initInterpreter() {
	log.Debug("Initialize Python")

	runtime.LockOSThread()

	if C.Py_IsInitialized() == 0 {
		C.Py_Initialize()
	}
	if C.Py_IsInitialized() == 0 {
		panic(fmt.Errorf("Could not initialize the python interpreter"))
	}

	// make sure the GIL is correctly initialized
	if C.PyEval_ThreadsInitialized() == 0 {
		C.PyEval_InitThreads()
	}
	if C.PyEval_ThreadsInitialized() == 0 {
		panic(fmt.Errorf("Could not initialize the GIL"))
	}
	log.Debug("Initialized Python.")
	_tstate := C.PyGILState_GetThisThreadState()
	C.PyEval_ReleaseThread(_tstate)
}

func run_check(module string, args []string, kw map[string]string, agentCheckClass *python.PyObject) string {
	// Log and release the GIL at the end of the run
	_gstate := C.PyGILState_Ensure()
	defer func() {
		C.PyGILState_Release(_gstate)
	}()

	// import python module
	_module := python.PyImport_ImportModuleNoBlock(module)
	if _module == nil {
		log.Errorf("Unable to load %v", module)
		return ""
	}

	// Try to found a class inhereting from AgentCheck
	dir := _module.PyObject_Dir()
	var _klass *python.PyObject
	classFound := false
	for i := 0; i < python.PyList_GET_SIZE(dir); i++ {
		_klass_name := python.PyString_AsString(python.PyList_GET_ITEM(dir, i))
		_klass = _module.GetAttrString(_klass_name)

		if python.PyClass_IsSubclass(_klass, agentCheckClass) && _klass_name != "AgentCheck" {
			log.Infof("Found %v", _klass_name)
			classFound = true
			break
		}
	}

	if !classFound {
		log.Infof("No class found for module %v", module)
		return ""
	}
	_instance := _klass.CallObject(python.PyTuple_New(0))

	// convert golang arguments to python argument.
	_a := python.PyTuple_New(len(args))

	for i, v := range args {
		python.PyTuple_SET_ITEM(_a, i, python.PyString_FromString(v))

	}

	_kw := python.PyDict_New()
	for k, v := range kw {
		python.PyDict_SetItem(
			_kw, python.PyString_FromString(k),
			python.PyString_FromString(v),
		)
	}

	// pack arguments
	_args := python.PyTuple_New(2)
	python.PyTuple_SET_ITEM(_args, 0, _a)
	python.PyTuple_SET_ITEM(_args, 1, _kw)

	// call run function
	_run_fct := _instance.GetAttrString("run")
	_result := _run_fct.CallObject(_args)
	_result_string := python.PyString_AsString(_result)
	return _result_string
}

var log = logging.MustGetLogger("datadog-agent")

func StartLoop() {
	initInterpreter()

	// create argument
	_kw := map[string]string{}

	_modules := []string{"checks.system_core", "checks.ntp"}

	_gstate := C.PyGILState_Ensure()
	_agentCheckClass := python.PyImport_ImportModuleNoBlock("checks").GetAttrString("AgentCheck")
	C.PyGILState_Release(_gstate)

	for i := 0; i < 10000000; i++ {
		for _, module := range _modules {
			log.Infof("Running check %v", module)

			results := run_check(module, []string{"instance1"}, _kw, _agentCheckClass)
			log.Infof("Result: %v", results)

		}
		log.Info("Sleeping for 10secs")
		time.Sleep(10 * time.Second)

	}

}
