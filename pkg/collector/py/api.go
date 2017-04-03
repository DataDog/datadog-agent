package py

import (
	"errors"
	"unsafe"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// #cgo pkg-config: python-2.7
// #cgo windows LDFLAGS: -Wl,--allow-multiple-definition
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
	_tags, err := extractTags(tags)
	if err != nil {
		return nil
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
	_tags, err := extractTags(tags)
	if err != nil {
		return nil
	}
	_message := C.GoString(message)

	sender.ServiceCheck(_name, _status, "", _tags, _message)

	return C._none()
}

// SubmitEvent is the method exposed to Python scripts to submit events
//export SubmitEvent
func SubmitEvent(check *C.PyObject, event *C.PyObject) *C.PyObject {

	sender, err := aggregator.GetDefaultSender()
	if err != nil {
		log.Errorf("Error submitting event to the Sender: %v", err)
		return C._none()
	}

	if int(C._PyDict_Check(event)) == 0 {
		log.Errorf("Error submitting event to the Sender, the submitted event is not a python dict")
		return C._none()
	}

	_event, err := extractEventFromDict(event)
	if err != nil {
		return nil
	}

	sender.Event(_event)

	return C._none()
}

func extractEventFromDict(event *C.PyObject) (aggregator.Event, error) {
	// Extract all string values
	// Values that should be extracted from the python event dict as strings
	eventStringValues := map[string]string{
		"msg_title":        "",
		"msg_text":         "",
		"priority":         "",
		"host":             "",
		"alert_type":       "",
		"aggregation_key":  "",
		"source_type_name": "",
	}

	for key := range eventStringValues {
		pyKey := C.CString(key)
		defer C.free(unsafe.Pointer(pyKey))

		pyValue := C.PyDict_GetItemString(event, pyKey) // borrowed ref
		// key not in dict => nil ; value for key is None => None ; we need to check for both
		if pyValue != nil && !isNone(pyValue) {
			if int(C._PyString_Check(pyValue)) != 0 {
				eventStringValues[key] = C.GoString(C.PyString_AsString(pyValue))
			} else {
				log.Infof("Can't parse value for key '%s' in event submitted from python check", key)
			}
		}
	}

	_event := aggregator.Event{
		Title:          eventStringValues["msg_title"],
		Text:           eventStringValues["msg_text"],
		Priority:       aggregator.EventPriority(eventStringValues["priority"]),
		Host:           eventStringValues["host"],
		AlertType:      aggregator.EventAlertType(eventStringValues["alert_type"]),
		AggregationKey: eventStringValues["aggregation_key"],
		SourceTypeName: eventStringValues["source_type_name"],
	}

	// Extract timestamp
	pyKey := C.CString("timestamp")
	defer C.free(unsafe.Pointer(pyKey))

	timestamp := C.PyDict_GetItemString(event, pyKey) // borrowed ref
	if timestamp != nil && !isNone(timestamp) {
		_event.Ts = int64(C.PyInt_AsLong(timestamp))
	}

	// Extract tags
	pyKey = C.CString("tags")
	defer C.free(unsafe.Pointer(pyKey))

	tags := C.PyDict_GetItemString(event, pyKey) // borrowed ref
	if tags != nil {
		_tags, err := extractTags(tags)
		if err != nil {
			return _event, err
		}
		_event.Tags = _tags
	}

	return _event, nil
}

func extractTags(tags *C.PyObject) (_tags []string, err error) {
	if !isNone(tags) {
		errMsg := C.CString("expected tags to be a sequence")
		defer C.free(unsafe.Pointer(errMsg))

		var seq *C.PyObject
		seq = C.PySequence_Fast(tags, errMsg) // seq is a new reference, has to be decref'd
		if seq == nil {
			err = errors.New("not a sequence")
			return
		}
		defer C.Py_DecRef(seq)

		var i C.Py_ssize_t
		for i = 0; i < C.PySequence_Fast_Get_Size(seq); i++ {
			item := C.PySequence_Fast_Get_Item(seq, i)                   // `item` is borrowed, no need to decref
			_tags = append(_tags, C.GoString(C.PyString_AsString(item))) // TODO: YOLO! Please add error checking
		}
	}

	return
}

func isNone(o *C.PyObject) bool {
	return int(C._is_none(o)) != 0
}

func initAPI() {
	C.initaggregator()
}
