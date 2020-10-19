// +build cpython

package py

import (
	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	chk "github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"github.com/mitchellh/mapstructure"
)

// #cgo pkg-config: python-2.7
// #cgo windows LDFLAGS: -Wl,--allow-multiple-definition
// #include "telemetry_api.h"
// #include "topology_api.h"
// #include "api.h"
// #include "stdlib.h"
// #include <Python.h>
import "C"

// SubmitTopologyEvent is the method exposed to Python scripts to submit contextualized events
//export SubmitTopologyEvent
func SubmitTopologyEvent(check *C.PyObject, checkID *C.char, event *C.PyObject) *C.PyObject {

	goCheckID := C.GoString(checkID)
	var sender aggregator.Sender
	var err error

	sender, err = aggregator.GetSender(chk.ID(goCheckID))

	if err != nil || sender == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return C._none()
	}

	if int(C._PyDict_Check(event)) == 0 {
		log.Errorf("Error submitting event to the Sender, the submitted event is not a python dict")
		return C._none()
	}

	eventMap, err := extractStructureFromObject(event, goCheckID)
	if err != nil {
		log.Error(err)
		return nil
	}

	var topologyEvent metrics.Event
	err = mapstructure.Decode(eventMap, &topologyEvent)
	if err != nil {
		log.Error(err)
		return nil
	}

	sender.Event(topologyEvent)

	return C._none()
}

func initTelemetry() {
	C.inittelemetry()
}
