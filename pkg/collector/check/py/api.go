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

// SubmitMetric is the method exposed to Python scripts to submit metrics
//export SubmitMetric
func SubmitMetric(check *C.PyObject, mt C.MetricType, name *C.char, value C.float, tags *C.PyObject) *C.PyObject {

	sender, err := aggregator.GetDefaultSender()
	if err != nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
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
	case C.GAUGE:
		sender.Gauge(_name, _value, "", _tags)
	case C.RATE:
		sender.Rate(_name, _value, "", _tags)
	case C.COUNT:
		sender.Count(_name, _value, "", _tags)
	case C.MONOTONIC_COUNT:
		sender.MonotonicCount(_name, _value, "", _tags)
	case C.HISTOGRAM:
		sender.Histogram(_name, _value, "", _tags)
	}

	// cleanup
	C.Py_DecRef(seq)
	C.free(unsafe.Pointer(errMsg))

	return C._none()
}

// SubmitServiceCheck is the method exposed to Python scripts to submit service checks
//export SubmitServiceCheck
func SubmitServiceCheck(check *C.PyObject, name *C.char, status C.int, tags *C.PyObject, message *C.char) *C.PyObject {

	sender, err := aggregator.GetDefaultSender()
	if err != nil {
		log.Errorf("Error submitting service check to the Sender: %v", err)
		return C._none()
	}

	_name := C.GoString(name)
	_status := aggregator.ServiceCheckStatus(status)
	var _tags []string
	var seq *C.PyObject
	_message := C.GoString(message)

	errMsg := C.CString("expected a sequence") // this has to be freed
	seq = C.PySequence_Fast(tags, errMsg)      // seq is a new reference, has to be decref'd
	var i C.Py_ssize_t
	for i = 0; i < C.PySequence_Fast_Get_Size(seq); i++ {
		item := C.PySequence_Fast_Get_Item(seq, i)                   // `item` is borrowed, no need to decref
		_tags = append(_tags, C.GoString(C.PyString_AsString(item))) // TODO: YOLO! Please add error checking
	}

	sender.ServiceCheck(_name, _status, "", _tags, _message)

	// cleanup
	C.Py_DecRef(seq)
	C.free(unsafe.Pointer(errMsg))

	return C._none()
}

func initAPI() {
	C.initaggregator()
}
