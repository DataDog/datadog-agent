// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package py

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// #cgo pkg-config: python-2.7
// #cgo linux CFLAGS: -std=gnu99
// #include "api.h"
// #include "datadog_agent.h"
import "C"

// GetVersion expose the version of the agent to python check (used as a PyCFunction in the datadog_agent python module)
//export GetVersion
func GetVersion(self *C.PyObject, args *C.PyObject) *C.PyObject {
	av, _ := version.New(version.AgentVersion)

	cStr := C.CString(av.GetNumber())
	pyStr := C.PyString_FromString(cStr)
	C.free(unsafe.Pointer(cStr))
	return pyStr
}

// GetHostname expose the current hostname of the agent to python check (used as a PyCFunction in the datadog_agent python module)
//export GetHostname
func GetHostname(self *C.PyObject, args *C.PyObject) *C.PyObject {
	hostname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err)
		hostname = ""
	}

	cStr := C.CString(hostname)
	pyStr := C.PyString_FromString(cStr)
	C.free(unsafe.Pointer(cStr))
	return pyStr
}

// Headers return HTTP headers with basic information like UserAgent already set (used as a PyCFunction in the datadog_agent python module)
//export Headers
func Headers(self *C.PyObject, args *C.PyObject) *C.PyObject {
	h := util.HTTPHeaders()

	dict := C.PyDict_New()
	for k, v := range h {
		cKey := C.CString(k)
		pyKey := C.PyString_FromString(cKey)
		defer C.Py_DecRef(pyKey)
		C.free(unsafe.Pointer(cKey))

		cVal := C.CString(v)
		pyVal := C.PyString_FromString(cVal)
		defer C.Py_DecRef(pyVal)
		C.free(unsafe.Pointer(cVal))

		C.PyDict_SetItem(dict, pyKey, pyVal)
	}
	return dict
}

// GetConfig returns a value from the agent configuration.
//export GetConfig
func GetConfig(key *C.char) *C.PyObject {
	goKey := C.GoString(key)
	if !config.Datadog.IsSet(goKey) {
		return C._none()
	}

	value := config.Datadog.Get(goKey)
	pyValue, err := ToPython(value)
	if err != nil {
		log.Errorf("datadog_agent: could not convert configuration value (%v) to python types: %s", value, err)
		return C._none()
	}
	// converting type *python.C.struct__object to *C.struct__object
	return (*C.PyObject)(unsafe.Pointer(pyValue.GetCPointer()))
}

// LogMessage logs a message from python through the agent logger (see
// https://docs.python.org/2.7/library/logging.html#logging-levels)
//export LogMessage
func LogMessage(message *C.char, logLevel C.int) *C.PyObject {
	goMsg := C.GoString(message)

	switch logLevel {
	case 50: // CRITICAL
		log.Critical(goMsg)
	case 40: // ERROR
		log.Error(goMsg)
	case 30: // WARNING
		log.Warn(goMsg)
	case 20: // INFO
		log.Info(goMsg)
	case 10: // DEBUG
		log.Debug(goMsg)
	default: // unknown log level
		log.Info(goMsg)
	}

	return C._none()
}

// GetSubprocessOutput runs the subprocess and returns the output
//export GetSubprocessOutput
func GetSubprocessOutput(argv **C.char, argc, raise int) *C.PyObject {

	// IMPORTANT: this is (probably) running in a go routine already locked to
	//            a thread. No need to do it again, and definitely no need to
	//            to release it - we can let the caller do that.

	// https://github.com/golang/go/wiki/cgo#turning-c-arrays-into-go-slices
	length := int(argc)
	subprocessArgs := make([]string, length-1)
	cmdSlice := (*[1 << 30]*C.char)(unsafe.Pointer(argv))[:length:length]
	subprocessCmd := C.GoString(cmdSlice[0])
	for i := 1; i < length; i++ {
		subprocessArgs[i-1] = C.GoString(cmdSlice[i])
	}
	cmd := exec.Command(subprocessCmd, subprocessArgs...)

	glock := C.PyGILState_Ensure()
	defer C.PyGILState_Release(glock)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cErr := C.CString(fmt.Sprintf("internal error creating stdout pipe: %v", err))
		C.PyErr_SetString(C.PyExc_Exception, cErr)
		C.free(unsafe.Pointer(cErr))
		return C._none()
	}

	var wg sync.WaitGroup
	var output []byte
	wg.Add(1)
	go func() {
		defer wg.Done()
		output, _ = ioutil.ReadAll(stdout)
	}()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cErr := C.CString(fmt.Sprintf("internal error creating stderr pipe: %v", err))
		C.PyErr_SetString(C.PyExc_Exception, cErr)
		C.free(unsafe.Pointer(cErr))
		return C._none()
	}

	var outputErr []byte
	wg.Add(1)
	go func() {
		defer wg.Done()
		outputErr, _ = ioutil.ReadAll(stderr)
	}()

	cmd.Start()

	retCode := 0
	err = cmd.Wait()
	if exiterr, ok := err.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			retCode = status.ExitStatus()
		}
	}
	wg.Wait()

	if raise > 0 {
		// raise on error
		if len(output) == 0 {
			cModuleName := C.CString("util")
			utilModule := C.PyImport_ImportModule(cModuleName)
			C.free(unsafe.Pointer(cModuleName))
			if utilModule == nil {
				cErr := C.CString("unable to import subprocess empty output exception")
				C.PyErr_SetString(C.PyExc_Exception, cErr)
				C.free(unsafe.Pointer(cErr))
				return C._none()
			}
			defer C.Py_DecRef(utilModule)

			cExcName := C.CString("SubprocessOutputEmptyError")
			excClass := C.PyObject_GetAttrString(utilModule, cExcName)
			C.free(unsafe.Pointer(cExcName))
			if excClass == nil {
				cErr := C.CString("unable to import subprocess empty output exception")
				C.PyErr_SetString(C.PyExc_Exception, cErr)
				C.free(unsafe.Pointer(cErr))
				return C._none()
			}
			defer C.Py_DecRef(excClass)

			cErr := C.CString("get_subprocess_output expected output but had none.")
			C.PyErr_SetString((*C.PyObject)(unsafe.Pointer(excClass)), cErr)
			C.free(unsafe.Pointer(cErr))
			return C._none()
		}
	}

	cOutput := C.CString(string(output[:]))
	pyOutput := C.PyString_FromString(cOutput)
	C.free(unsafe.Pointer(cOutput))
	cOutputErr := C.CString(string(outputErr[:]))
	pyOutputErr := C.PyString_FromString(cOutputErr)
	C.free(unsafe.Pointer(cOutputErr))
	pyRetCode := C.PyInt_FromLong(C.long(retCode))

	pyResult := C.PyTuple_New(3)
	C.PyTuple_SetItem(pyResult, 0, pyOutput)
	C.PyTuple_SetItem(pyResult, 1, pyOutputErr)
	C.PyTuple_SetItem(pyResult, 2, pyRetCode)

	return pyResult
}

func initDatadogAgent() {
	C.initdatadogagent()
}
