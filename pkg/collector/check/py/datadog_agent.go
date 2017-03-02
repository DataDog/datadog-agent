package py

import (
	"unsafe"

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

func initDatadogAgent() {
	C.initdatadogagent()
}
