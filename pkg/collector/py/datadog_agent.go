package py

import (
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

// GetVersion expose the version of the agent to python check
//export GetVersion
func GetVersion() *C.PyObject {
	av, _ := version.New(version.AgentVersion)

	cStr := C.CString(av.GetNumber())
	pyStr := C.PyString_FromString(cStr)
	C.free(unsafe.Pointer(cStr))
	return pyStr
}

// Headers return HTTP headers with basic information like UserAgent already set
//export Headers
func Headers() *C.PyObject {
	h := util.HTTPHeaders()

	dict := C.PyDict_New()
	for k, v := range h {
		cKey := C.CString(k)
		pyKey := C.PyString_FromString(cKey)
		C.free(unsafe.Pointer(cKey))

		cVal := C.CString(v)
		pyVal := C.PyString_FromString(cVal)
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

func initDatadogAgent() {
	C.initdatadogagent()
}
