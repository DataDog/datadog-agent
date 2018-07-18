// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package py

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// #cgo pkg-config: python-2.7
// #cgo linux CFLAGS: -std=gnu99
// #include "api.h"
// #include "datadog_agent.h"
import "C"

// GetVersion exposes the version of the agent to Python checks.
// Used as a PyCFunction of type METH_VARARGS mapped to `datadog_agent.get_version`.
// `self` is the module object.
//export GetVersion
func GetVersion(self *C.PyObject, args *C.PyObject) *C.PyObject {
	av, _ := version.New(version.AgentVersion, version.Commit)

	cStr := C.CString(av.GetNumber())
	pyStr := C.PyString_FromString(cStr)
	C.free(unsafe.Pointer(cStr))
	return pyStr
}

// GetHostname exposes the current hostname of the agent to Python checks.
// Used as a PyCFunction of type METH_VARARGS mapped to `datadog_agent.get_hostname`.
// `self` is the module object.
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

// GetClustername exposes the current clustername (if it exists) of the agent to Python checks.
// Used as a PyCFunction of type METH_VARARGS mapped to `datadog_agent.get_clustername`.
// `self` is the module object.
//export GetClusterName
func GetClusterName(self *C.PyObject, args *C.PyObject) *C.PyObject {
	clusterName := clustername.GetClusterName()

	cStr := C.CString(clusterName)
	pyStr := C.PyString_FromString(cStr)
	C.free(unsafe.Pointer(cStr))
	return pyStr
}

// Headers returns a basic set of HTTP headers that can be used by clients in Python checks.
// Used as a PyCFunction of type METH_KEYWORDS mapped to `datadog_agent.headers`.
// `self` is the module object.
//export Headers
func Headers(self *C.PyObject, args, kwargs *C.PyObject) *C.PyObject {
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

	// some checks need to add an extra header when they pass `http_host`
	if kwargs != nil {
		cKey := C.CString("http_host")
		// in case of failure, the following doesn't set an exception
		// pyHttpHost is borrowed
		pyHTTPHost := C.PyDict_GetItemString(kwargs, cKey)
		C.free(unsafe.Pointer(cKey))
		if pyHTTPHost != nil {
			// set the Host header
			cKey = C.CString("Host")
			C.PyDict_SetItemString(dict, cKey, pyHTTPHost)
			C.free(unsafe.Pointer(cKey))
		}
	}

	return dict
}

// GetConfig returns a value from the agent configuration.
// Indirectly used by the C function `get_config` that's mapped to `datadog_agent.get_config`.
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
// Indirectly used by the C function `log_message` that's mapped to `datadog_agent.log`.
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
// Indirectly used by the C function `get_subprocess_output` that's mapped to `_util.get_subprocess_output`.
//export GetSubprocessOutput
func GetSubprocessOutput(argv **C.char, argc, raise int) *C.PyObject {

	// IMPORTANT: this is (probably) running in a go routine already locked to
	//            a thread. No need to do it again, and definitely no need to
	//            to release it - we can let the caller do that.

	// https://github.com/golang/go/wiki/cgo#turning-c-arrays-into-go-slices

	threadState := SaveThreadState()

	length := int(argc)
	subprocessArgs := make([]string, length-1)
	cmdSlice := (*[1 << 30]*C.char)(unsafe.Pointer(argv))[:length:length]
	subprocessCmd := C.GoString(cmdSlice[0])
	for i := 1; i < length; i++ {
		subprocessArgs[i-1] = C.GoString(cmdSlice[i])
	}
	cmd := exec.Command(subprocessCmd, subprocessArgs...)

	cmdKey := fmt.Sprintf("%s-%v", cmd.Path, time.Now().UnixNano())
	runningProcesses.Add(cmdKey, cmd)
	defer runningProcesses.Remove(cmdKey)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		glock := RestoreThreadStateAndLock(threadState)
		defer C.PyGILState_Release(glock)

		cErr := C.CString(fmt.Sprintf("internal error creating stdout pipe: %v", err))
		C.PyErr_SetString(C.PyExc_Exception, cErr)
		C.free(unsafe.Pointer(cErr))
		return nil
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
		glock := RestoreThreadStateAndLock(threadState)
		defer C.PyGILState_Release(glock)

		cErr := C.CString(fmt.Sprintf("internal error creating stderr pipe: %v", err))
		C.PyErr_SetString(C.PyExc_Exception, cErr)
		C.free(unsafe.Pointer(cErr))
		return nil
	}

	var outputErr []byte
	wg.Add(1)
	go func() {
		defer wg.Done()
		outputErr, _ = ioutil.ReadAll(stderr)
	}()

	cmd.Start()

	// Wait for the pipes to be closed *before* waiting for the cmd to exit, as per os.exec docs
	wg.Wait()

	retCode := 0
	err = cmd.Wait()
	if exiterr, ok := err.(*exec.ExitError); ok {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
			retCode = status.ExitStatus()
		}
	}

	glock := RestoreThreadStateAndLock(threadState)
	defer C.PyGILState_Release(glock)

	if raise > 0 {
		// raise on error
		if len(output) == 0 {
			cModuleName := C.CString("_util")
			utilModule := C.PyImport_ImportModule(cModuleName)
			C.free(unsafe.Pointer(cModuleName))
			if utilModule == nil {
				return nil
			}
			defer C.Py_DecRef(utilModule)

			cExcName := C.CString("SubprocessOutputEmptyError")
			excClass := C.PyObject_GetAttrString(utilModule, cExcName)
			C.free(unsafe.Pointer(cExcName))
			if excClass == nil {
				return nil
			}
			defer C.Py_DecRef(excClass)

			cErr := C.CString("get_subprocess_output expected output but had none.")
			C.PyErr_SetString((*C.PyObject)(unsafe.Pointer(excClass)), cErr)
			C.free(unsafe.Pointer(cErr))
			return nil
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

// SetExternalTags adds a set of tags for a given hostnane to the External Host
// Tags metadata provider cache.
// Indirectly used by the C function `set_external_tags` that's mapped to `datadog_agent.set_external_tags`.
//export SetExternalTags
func SetExternalTags(hostname, sourceType *C.char, tags **C.char, tagsLen C.int) *C.PyObject {
	hname := C.GoString(hostname)
	stype := C.GoString(sourceType)
	tlen := int(tagsLen)
	tagsSlice := (*[1 << 30]*C.char)(unsafe.Pointer(tags))[:tlen:tlen]
	tagsStrings := []string{}

	for i := 0; i < tlen; i++ {
		tag := C.GoString(tagsSlice[i])
		tagsStrings = append(tagsStrings, tag)
	}

	externalhost.SetExternalTags(hname, stype, tagsStrings)
	return C._none()
}

func initDatadogAgent() {
	C.initdatadogagent()
}
