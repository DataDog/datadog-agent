// +build python,test

package python

import (
	"encoding/json"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/health"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

// #include <datadog_agent_rtloader.h>
import "C"

func testComponentTopology(t *testing.T) {
	mockBatcher := batcher.NewMockBatcher()

	c := &topology.Component{
		ExternalID: "external-id",
		Type:       topology.Type{Name: "component-type"},
		Data: map[string]interface{}{
			"some": "data",
		},
	}
	data, err := json.Marshal(c)
	assert.NoError(t, err)

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
		C.CString(string(data)))
	SubmitStopSnapshot(checkId, &instanceKey)

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	instance := topology.Instance{Type: "instance-type", URL: "instance-url"}

	assert.Equal(t, batcher.CheckInstanceBatchStates(map[check.ID]batcher.CheckInstanceBatchState{
		"check-id": {
			Health: make(map[string]health.Health),
			Topology: &topology.Topology{
				StartSnapshot: true,
				StopSnapshot:  true,
				Instance:      instance,
				Components: []topology.Component{
					{
						ExternalID: "external-id",
						Type:       topology.Type{Name: "component-type"},
						Data:       topology.Data{"some": "data"},
					},
				},
				Relations: []topology.Relation{},
			},
		},
	}), expectedTopology)
}

func testRelationTopology(t *testing.T) {
	mockBatcher := batcher.NewMockBatcher()

	c := &topology.Relation{
		SourceID: "source-id",
		TargetID: "target-id",
		Type:     topology.Type{Name: "relation-type"},
		Data: map[string]interface{}{
			"some": "data",
		},
	}
	data, err := json.Marshal(c)
	assert.NoError(t, err)

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
		C.CString(string(data)))

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	instance := topology.Instance{Type: "instance-type", URL: "instance-url"}

	assert.Equal(t, batcher.CheckInstanceBatchStates(map[check.ID]batcher.CheckInstanceBatchState{
		"check-id": {
			Health: make(map[string]health.Health),
			Topology: &topology.Topology{
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
						Data:       topology.Data{"some": "data"},
					},
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

	assert.Equal(t, batcher.CheckInstanceBatchStates(map[check.ID]batcher.CheckInstanceBatchState{
		"check-id": {
			Health: make(map[string]health.Health),
			Topology: &topology.Topology{
				StartSnapshot: true,
				StopSnapshot:  false,
				Instance:      instance,
				Components:    []topology.Component{},
				Relations:     []topology.Relation{},
			},
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

	assert.Equal(t, batcher.CheckInstanceBatchStates(map[check.ID]batcher.CheckInstanceBatchState{
		"check-id": {
			Health: make(map[string]health.Health),
			Topology: &topology.Topology{
				StartSnapshot: false,
				StopSnapshot:  true,
				Instance:      instance,
				Components:    []topology.Component{},
				Relations:     []topology.Relation{},
			},
		},
	}), expectedTopology)
}
