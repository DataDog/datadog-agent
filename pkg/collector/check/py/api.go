package py

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// #cgo pkg-config: python-2.7
// #cgo linux CFLAGS: -std=gnu99
// #include "api.h"
import "C"

// SubmitData is the method exposed to Python scripts
//export SubmitData
func SubmitData(check *C.PyObject, mt C.MetricType, name *C.char, value C.float, tags *C.PyObject) *C.PyObject {

	sender, err := aggregator.GetDefaultSender()
	if err != nil {
		log.Errorf("Error submitting data to the Sender: %v", err)
		return C._none()
	}

	// TODO: cleanup memory, C.stuff is going to stay there!!!

	_name := C.GoString(name)
	_value := float64(value)
	var _tags []string
	var seq *C.PyObject

	seq = C.PySequence_Fast(tags, C.CString("expected a sequence"))
	l := C.PySequence_Size(tags)
	var i C.Py_ssize_t
	for i = 0; i < l; i++ {
		item := C.PySequence_Fast_Get_Item(seq, i)
		_tags = append(_tags, C.GoString(C.PyString_AsString(item))) // YOLO! Please remove
	}
	C.Py_DecRef(seq)

	switch mt {
	case C.RATE:
		sender.Rate(_name, _value, "", _tags)
	case C.GAUGE:
		sender.Gauge(_name, _value, "", _tags)
	case C.HISTOGRAM:
		sender.Histogram(_name, _value, "", _tags)
	}

	return C._none()
}

func initAPI() {
	C.initaggregator()
}
