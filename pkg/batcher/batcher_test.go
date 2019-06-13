package batcher

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	serializer2 "github.com/StackVista/stackstate-agent/pkg/serializer"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	testInstance  = topology.Instance{Type: "mytype", Url: "myurl"}
	testHost      = "myhost"
	testAgent     = "myagent"
	testId        = check.ID("myid")
	testId2       = check.ID("myid2")
	testComponent = topology.Component{
		ExternalId: "id",
		Type:       topology.Type{Name: "typename"},
		Data:       map[string]interface{}{},
	}
	testComponent2 = topology.Component{
		ExternalId: "id2",
		Type:       topology.Type{Name: "typename"},
		Data:       map[string]interface{}{},
	}
	testRelation = topology.Relation{
		ExternalId: "id2",
		Type:       topology.Type{Name: "typename"},
		SourceId:   "source",
		TargetId:   "target",
		Data:       map[string]interface{}{},
	}
)

func TestBatchFlushOnStop(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 100)

	batcher.SubmitStopSnapshot(testId, testInstance)

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

func TestBatchFlushOnCommit(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 100)

	batcher.SubmitComponent(testId, testInstance, testComponent)

	batcher.SubmitComplete(testId)

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

func TestBatchNoDataNoCommit(t *testing.T) {
	serializer := serializer2.NewAgentV1MockSerializer()
	batcher := newAsynchronousBatcher(serializer, testHost, testAgent, 100)

	batcher.SubmitComponent(testId, testInstance, testComponent)

	batcher.SubmitComplete(testId2)

	// We now send a stop to trigger a combined commit
	batcher.SubmitStopSnapshot(testId, testInstance)

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

	batcher.SubmitComponent(testId, testInstance, testComponent)
	batcher.SubmitComponent(testId, testInstance, testComponent2)

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

	batcher.SubmitStartSnapshot(testId, testInstance)
	batcher.SubmitComplete(testId)

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

	batcher.SubmitRelation(testId, testInstance, testRelation)
	batcher.SubmitComplete(testId)

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
