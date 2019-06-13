package batcher

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/serializer"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"sync"
)

var (
	batcherInstance Batcher
	batcherInit     sync.Once
)

// Batcher interface can receive data for sending to the intake and will accumulate the data in batches. This does
// not work on a fixed schedule like the aggregator but flushes either when data exceeds a threshold, when
// data is complete.
type Batcher interface {
	SubmitComponent(checkID check.ID, instance topology.Instance, component topology.Component)
	SubmitRelation(checkID check.ID, instance topology.Instance, relation topology.Relation)
	SubmitStartSnapshot(checkID check.ID, instance topology.Instance)
	SubmitStopSnapshot(checkID check.ID, instance topology.Instance)
	SubmitComplete(checkID check.ID)
	Shutdown()
}

// InitBatcher initializes the global batcher instance
func InitBatcher(serializer serializer.AgentV1Serializer, hostname, agentName string, batchLimit int) {
	batcherInit.Do(func() {
		batcherInstance = newAsynchronousBatcher(serializer, hostname, agentName, batchLimit)
	})
}

func newAsynchronousBatcher(serializer serializer.AgentV1Serializer, hostname, agentName string, batchLimit int) AsynchronousBatcher {
	batcher := AsynchronousBatcher{
		builder:    NewTopologyBuilder(batchLimit),
		hostname:   hostname,
		agentName:  agentName,
		input:      make(chan interface{}),
		serializer: serializer,
	}
	go batcher.run()
	return batcher
}

// GetBatcher returns a handle on the global batcher instance
func GetBatcher() Batcher {
	return batcherInstance
}

// NewMockBatcher initializes the global batcher with a mock version, intended for testing
func NewMockBatcher() MockBatcher {
	batcher := createMockBatcher()
	batcherInstance = batcher
	return batcher
}

// AsynchronousBatcher is the implementation of the batcher. Works asynchronous. Publishes data to the serializer
type AsynchronousBatcher struct {
	builder             TopologyBuilder
	hostname, agentName string
	input               chan interface{}
	serializer          serializer.AgentV1Serializer
}

type submitComponent struct {
	checkID   check.ID
	instance  topology.Instance
	component topology.Component
}

type submitRelation struct {
	checkID  check.ID
	instance topology.Instance
	relation topology.Relation
}

type submitStartSnapshot struct {
	checkID  check.ID
	instance topology.Instance
}

type submitStopSnapshot struct {
	checkID  check.ID
	instance topology.Instance
}

type submitComplete struct {
	checkID check.ID
}

type submitShutdown struct{}

func (batcher *AsynchronousBatcher) sendTopology(topologyMap map[check.ID]topology.Topology) {
	if topologyMap != nil {

		topologies := make([]topology.Topology, len(topologyMap))
		idx := 0
		for _, topo := range topologyMap {
			topologies[idx] = topo
			idx++
		}

		payload := map[string]interface{}{
			"internalHostname": batcher.hostname,
			"topologies":       topologies,
		}

		batcher.serializer.SendJSONToV1Intake(payload)
	}
}

func (batcher *AsynchronousBatcher) run() {
	for {
		s := <-batcher.input
		switch submission := s.(type) {
		case submitComponent:
			batcher.sendTopology(batcher.builder.AddComponent(submission.checkID, submission.instance, submission.component))
		case submitRelation:
			batcher.sendTopology(batcher.builder.AddRelation(submission.checkID, submission.instance, submission.relation))
		case submitStartSnapshot:
			batcher.sendTopology(batcher.builder.StartSnapshot(submission.checkID, submission.instance))
		case submitStopSnapshot:
			batcher.sendTopology(batcher.builder.StopSnapshot(submission.checkID, submission.instance))
		case submitComplete:
			batcher.sendTopology(batcher.builder.FlushIfDataProduced(submission.checkID))
		case submitShutdown:
			return
		default:
			panic(fmt.Sprint("Unknown submission type"))
		}
	}
}

// SubmitComponent submits a component to the batch
func (batcher AsynchronousBatcher) SubmitComponent(checkID check.ID, instance topology.Instance, component topology.Component) {
	batcher.input <- submitComponent{
		checkID:   checkID,
		instance:  instance,
		component: component,
	}
}

// SubmitRelation submits a relation to the batch
func (batcher AsynchronousBatcher) SubmitRelation(checkID check.ID, instance topology.Instance, relation topology.Relation) {
	batcher.input <- submitRelation{
		checkID:  checkID,
		instance: instance,
		relation: relation,
	}
}

// SubmitStartSnapshot submits start of a snapshot
func (batcher AsynchronousBatcher) SubmitStartSnapshot(checkID check.ID, instance topology.Instance) {
	batcher.input <- submitStartSnapshot{
		checkID:  checkID,
		instance: instance,
	}
}

// SubmitStopSnapshot submits a stop of a snapshot. This always causes a flush of the data downstream
func (batcher AsynchronousBatcher) SubmitStopSnapshot(checkID check.ID, instance topology.Instance) {
	batcher.input <- submitStopSnapshot{
		checkID:  checkID,
		instance: instance,
	}
}

// SubmitComplete signals completion of a check. May trigger a flush only if the check produced data
func (batcher AsynchronousBatcher) SubmitComplete(checkID check.ID) {
	batcher.input <- submitComplete{
		checkID: checkID,
	}
}

// Shutdown shuts down the batcher
func (batcher AsynchronousBatcher) Shutdown() {
	batcher.input <- submitShutdown{}
}
