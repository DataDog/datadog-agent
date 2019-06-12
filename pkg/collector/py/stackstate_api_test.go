package py

import (
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestComponentTopology(t *testing.T) {
	check, _ := getCheckInstance("testtopology", "TestComponentCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := check.Run()
	assert.Nil(t, err)

	assert.Equal(t, map[string]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance: topology.Instance{
				Type: "type",
				Url:  "url",
			},
			Components: []topology.Component{
				{
					ExternalId: "myid",
					Type:       topology.Type{Name: "mytype"},
					Data: map[string]interface{}{
						"emptykey": map[string]interface{}{},
						"key":      "value",
						"intlist":  []interface{}{int64(1)},
						"nestedobject": map[string]interface{}{
							"nestedkey": "nestedValue",
						},
					},
				},
			},
			Relations: []topology.Relation{},
		},
	}, mockBatcher.CollectedTopology.Flush())
}

func TestRelationTopology(t *testing.T) {
	check, _ := getCheckInstance("testtopology", "TestRelationCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := check.Run()
	assert.Nil(t, err)

	assert.Equal(t, map[string]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance: topology.Instance{
				Type: "type",
				Url:  "url",
			},
			Components: []topology.Component{},
			Relations: []topology.Relation{
				{
					ExternalId: "source-mytype-target",
					SourceId:   "source",
					TargetId:   "target",
					Type:       topology.Type{Name: "mytype"},
					Data: map[string]interface{}{
						"emptykey": map[string]interface{}{},
						"key":      "value",
						"intlist":  []interface{}{int64(1)},
						"nestedobject": map[string]interface{}{
							"nestedkey": "nestedValue",
						},
					},
				},
			},
		},
	}, mockBatcher.CollectedTopology.Flush())
}

func TestStartSnapshotCheck(t *testing.T) {
	check, _ := getCheckInstance("testtopology", "TestStartSnapshotCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := check.Run()
	assert.Nil(t, err)

	assert.Equal(t, map[string]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  false,
			Instance: topology.Instance{
				Type: "type",
				Url:  "url",
			},
			Components: []topology.Component{},
			Relations:  []topology.Relation{},
		},
	}, mockBatcher.CollectedTopology.Flush())
}

func TestStopSnapshotCheck(t *testing.T) {
	check, _ := getCheckInstance("testtopology", "TestStopSnapshotCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := check.Run()
	assert.Nil(t, err)

	assert.Equal(t, map[string]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: false,
			StopSnapshot:  true,
			Instance: topology.Instance{
				Type: "type",
				Url:  "url",
			},
			Components: []topology.Component{},
			Relations:  []topology.Relation{},
		},
	}, mockBatcher.CollectedTopology.Flush())
}
