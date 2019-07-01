package batcher

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	serializer2 "github.com/StackVista/stackstate-agent/pkg/serializer"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	testInstance  = topology.Instance{Type: "mytype", URL: "myurl"}
	testHost      = "myhost"
	testAgent     = "myagent"
	testID        = check.ID("myid")
	testID2       = check.ID("myid2")
	testComponent = topology.Component{
		ExternalID: "id",
		Type:       topology.Type{Name: "typename"},
		Data:       map[string]interface{}{},
	}
	testComponent2 = topology.Component{
		ExternalID: "id2",
		Type:       topology.Type{Name: "typename"},
		Data:       map[string]interface{}{},
	}
	testRelation = topology.Relation{
		ExternalID: "id2",
		Type:       topology.Type{Name: "typename"},
		SourceID:   "source",
		TargetID:   "target",
		Data:       map[string]interface{}{},
	}
)

func TestBatchFlushOnStop(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 100)

	batcher.SubmitStopSnapshot(testID, testInstance)

	message := serializer.GetJSONToV1IntakeMessage()

	assert.Equal(t, message,
		map[string]interface{}{
			"internalHostname": "myhost",
			"topologies": []topology.Topology{
				{
					StartSnapshot: false,
					StopSnapshot:  true,
					Instance:      testInstance,
					Components:    []topology.Component{},
					Relations:     []topology.Relation{},
				},
			},
		})

	batcher.Shutdown()
}

func TestBatchFlushOnComplete(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 100)

	batcher.SubmitComponent(testID, testInstance, testComponent)

	batcher.SubmitComplete(testID)

	message := serializer.GetJSONToV1IntakeMessage()

	assert.Equal(t, message,
		map[string]interface{}{
			"internalHostname": "myhost",
			"topologies": []topology.Topology{
				{
					StartSnapshot: false,
					StopSnapshot:  false,
					Instance:      testInstance,
					Components:    []topology.Component{testComponent},
					Relations:     []topology.Relation{},
				},
			},
		})

	batcher.Shutdown()
}

func TestBatchNoDataNoComplete(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 100)

	batcher.SubmitComponent(testID, testInstance, testComponent)

	batcher.SubmitComplete(testID2)

	// We now send a stop to trigger a combined commit
	batcher.SubmitStopSnapshot(testID, testInstance)

	message := serializer.GetJSONToV1IntakeMessage()

	assert.Equal(t, message,
		map[string]interface{}{
			"internalHostname": "myhost",
			"topologies": []topology.Topology{
				{
					StartSnapshot: false,
					StopSnapshot:  true,
					Instance:      testInstance,
					Components:    []topology.Component{testComponent},
					Relations:     []topology.Relation{},
				},
			},
		})

	batcher.Shutdown()
}

func TestBatchFlushOnMaxElements(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 2)

	batcher.SubmitComponent(testID, testInstance, testComponent)
	batcher.SubmitComponent(testID, testInstance, testComponent2)

	message := serializer.GetJSONToV1IntakeMessage()

	assert.Equal(t, message,
		map[string]interface{}{
			"internalHostname": "myhost",
			"topologies": []topology.Topology{
				{
					StartSnapshot: false,
					StopSnapshot:  false,
					Instance:      testInstance,
					Components:    []topology.Component{testComponent, testComponent2},
					Relations:     []topology.Relation{},
				},
			},
		})

	batcher.Shutdown()
}

func TestBatcherStartSnapshot(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 100)

	batcher.SubmitStartSnapshot(testID, testInstance)
	batcher.SubmitComplete(testID)

	message := serializer.GetJSONToV1IntakeMessage()

	assert.Equal(t, message,
		map[string]interface{}{
			"internalHostname": "myhost",
			"topologies": []topology.Topology{
				{
					StartSnapshot: true,
					StopSnapshot:  false,
					Instance:      testInstance,
					Components:    []topology.Component{},
					Relations:     []topology.Relation{},
				},
			},
		})

	batcher.Shutdown()
}

func TestBatcherRelation(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 100)

	batcher.SubmitRelation(testID, testInstance, testRelation)
	batcher.SubmitComplete(testID)

	message := serializer.GetJSONToV1IntakeMessage()

	assert.Equal(t, message,
		map[string]interface{}{
			"internalHostname": "myhost",
			"topologies": []topology.Topology{
				{
					StartSnapshot: false,
					StopSnapshot:  false,
					Instance:      testInstance,
					Components:    []topology.Component{},
					Relations:     []topology.Relation{testRelation},
				},
			},
		})

	batcher.Shutdown()
}
