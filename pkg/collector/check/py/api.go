package py

import (
	"unsafe"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// #cgo pkg-config: python-2.7
// #cgo linux CFLAGS: -std=gnu99
// #include "api.h"
// #include "stdlib.h"
import "C"

// SubmitData is the method exposed to Python scripts
//export SubmitData
func SubmitData(check *C.PyObject, mt C.MetricType, name *C.char, value C.float, tags *C.PyObject) *C.PyObject {

	sender, err := aggregator.GetDefaultSender()
	if err != nil {
		log.Errorf("Error submitting data to the Sender: %v", err)
		return C._none()
	}

	_name := C.GoString(name)
	_value := float64(value)
	var _tags []string
	var seq *C.PyObject

	errMsg := C.CString("expected a sequence") // this has to be freed
	seq = C.PySequence_Fast(tags, errMsg)      // seq is a new reference, has to be decref'd
	var i C.Py_ssize_t
	for i = 0; i < C.PySequence_Fast_Get_Size(seq); i++ {
		item := C.PySequence_Fast_Get_Item(seq, i)                   // `item` is borrowed, no need to decref
		_tags = append(_tags, C.GoString(C.PyString_AsString(item))) // TODO: YOLO! Please add error checking
	}

	switch mt {
	case C.RATE:
		sender.Rate(_name, _value, "", _tags)
	case C.GAUGE:
		sender.Gauge(_name, _value, "", _tags)
	case C.HISTOGRAM:
		sender.Histogram(_name, _value, "", _tags)
	}

	// cleanup
	C.Py_DecRef(seq)
	C.free(unsafe.Pointer(errMsg))

	return C._none()
}

func initAPI() {
	C.initaggregator()
}
