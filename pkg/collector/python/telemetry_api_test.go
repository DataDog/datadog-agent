// +build python,test

package python

import "testing"

func TestTopologyEvent(t *testing.T) {
	testTopologyEvent(t)
}

func TestTopologyEventMissingFields(t *testing.T) {
	testTopologyEventMissingFields(t)
}

func TestTopologyEventInvalidYaml(t *testing.T) {
	testTopologyEventInvalidYaml(t)
}

func TestTopologyEventWrongFields(t *testing.T) {
	testTopologyEventWrongFields(t)
}
