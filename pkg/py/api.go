package py

import (
	"fmt"

	"github.com/DataDog/datadog-agent/aggregator"
)

// #cgo pkg-config: python2
// #cgo linux CFLAGS: -std=gnu99
// #include "api.h"
import "C"

//export SubmitData
func SubmitData(check *C.PyObject, mt C.MetricType, name *C.char, value C.float, tags *C.PyObject) *C.PyObject {

	agg := aggregator.Get()

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
		fmt.Println("Submitting Rate to the aggregator...", _name, _value, _tags)
		fallthrough
	case C.GAUGE:
		fmt.Println("Submitting Gauge to the aggregator...", _name, _value, _tags)
		agg.Gauge(_name, _value, "", _tags)
	case C.HISTOGRAM:
		fmt.Println("Submitting Histogram to the aggregator...", _name, _value, _tags)
		agg.Histogram(_name, _value, "", _tags)
	}

	return C._none()
}

func InitApi(aggregatorInstance aggregator.Aggregator) {
	aggregator.Set(aggregatorInstance)
	C.initaggregator()
}
