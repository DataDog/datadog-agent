// +build python,test

package python

import (
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

// #include <datadog_agent_rtloader.h>
import "C"

const yamlData = `
key: value ®
stringlist: 
  - a
  - b
  - c
boollist:
  - true
  - false
intlist:
  - 1
doublelist:
  - 0.7
  - 1.42
emptykey: null
nestedobject:
  nestedkey: nestedValue
  animals:
    legs: dog
    wings: eagle
    tail: crocodile
`

var expectedTopoData = topology.Data{
	"key":          "value ®",
	"stringlist":   []interface{}{"a", "b", "c"},
	"boollist":     []interface{}{true, false},
	"intlist":      []interface{}{1},
	"doublelist":   []interface{}{0.7, 1.42},
	"emptykey":     nil,
	"nestedobject": map[string]interface{}{
		"nestedkey": "nestedValue",
		"animals": map[string]interface{}{
			"legs": "dog",
			"wings": "eagle",
			"tail": "crocodile",
		},
	},
}

func testComponentTopology(t *testing.T) {
	mockBatcher := batcher.NewMockBatcher()

	checkId := C.CString("check-id")
	instanceKey := C.instance_key_t{}
	instanceKey.type_ = C.CString("instance-type")
	instanceKey.url = C.CString("instance-url")
	SubmitStartSnapshot(checkId, &instanceKey)
	SubmitComponent(
		checkId,
		&instanceKey,
		C.CString("external-id"),
		C.CString("component-type"),
		C.CString(yamlData))
	SubmitStopSnapshot(checkId, &instanceKey)

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	instance := topology.Instance{Type: "instance-type", URL: "instance-url"}

	for _, topos := range expectedTopology {
		for _, c := range topos.Components {
			c.JSONString()
		}
	}
	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"check-id": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      instance,
			Components:    []topology.Component{
				{
					ExternalID: "external-id",
					Type:       topology.Type{Name: "component-type"},
					Data: 		expectedTopoData,
				},
			},
			Relations:     []topology.Relation{},
		},
	}), expectedTopology)
}

func testRelationTopology(t *testing.T) {
	mockBatcher := batcher.NewMockBatcher()

	checkId := C.CString("check-id")
	instanceKey := C.instance_key_t{}
	instanceKey.type_ = C.CString("instance-type")
	instanceKey.url = C.CString("instance-url")
	SubmitRelation(
		checkId,
		&instanceKey,
		C.CString("source-id"),
		C.CString("target-id"),
		C.CString("relation-type"),
		C.CString(yamlData))

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	instance := topology.Instance{Type: "instance-type", URL: "instance-url"}

	for _, topos := range expectedTopology {
		for _, r := range topos.Relations {
			r.JSONString()
		}
	}
	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"check-id": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      instance,
			Components:    []topology.Component{},
			Relations: []topology.Relation{
				{
					ExternalID: "source-id-relation-type-target-id",
					SourceID:   "source-id",
					TargetID:   "target-id",
					Type:       topology.Type{Name: "relation-type"},
					Data: expectedTopoData,
				},
			},
		},
	}), expectedTopology)
}

func testStartSnapshotCheck(t *testing.T) {
	mockBatcher := batcher.NewMockBatcher()

	checkId := C.CString("check-id")
	instanceKey := C.instance_key_t{}
	instanceKey.type_ = C.CString("instance-type")
	instanceKey.url = C.CString("instance-url")
	SubmitStartSnapshot(checkId, &instanceKey)

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	instance := topology.Instance{Type: "instance-type", URL: "instance-url"}

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"check-id": {
			StartSnapshot: true,
			StopSnapshot:  false,
			Instance:      instance,
			Components:    []topology.Component{},
			Relations:     []topology.Relation{},
		},
	}), expectedTopology)
}

func testStopSnapshotCheck(t *testing.T) {
	mockBatcher := batcher.NewMockBatcher()

	checkId := C.CString("check-id")
	instanceKey := C.instance_key_t{}
	instanceKey.type_ = C.CString("instance-type")
	instanceKey.url = C.CString("instance-url")
	SubmitStopSnapshot(checkId, &instanceKey)

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	instance := topology.Instance{Type: "instance-type", URL: "instance-url"}

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"check-id": {
			StartSnapshot: false,
			StopSnapshot:  true,
			Instance:      instance,
			Components:    []topology.Component{},
			Relations:     []topology.Relation{},
		},
	}), expectedTopology)
}
