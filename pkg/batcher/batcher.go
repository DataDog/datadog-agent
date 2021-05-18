package batcher

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/health"
	"github.com/StackVista/stackstate-agent/pkg/serializer"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
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
	// Topology
	SubmitComponent(checkID check.ID, instance topology.Instance, component topology.Component)
	SubmitRelation(checkID check.ID, instance topology.Instance, relation topology.Relation)
	SubmitStartSnapshot(checkID check.ID, instance topology.Instance)
	SubmitStopSnapshot(checkID check.ID, instance topology.Instance)

	// Health
	SubmitHealthCheckData(checkID check.ID, stream health.Stream, data health.CheckData)
	SubmitHealthStartSnapshot(checkID check.ID, stream health.Stream, intervalSeconds int, expirySeconds int)
	SubmitHealthStopSnapshot(checkID check.ID, stream health.Stream)

	// lifecycle
	SubmitComplete(checkID check.ID)
	Shutdown()
}

// InitBatcher initializes the global batcher instance
func InitBatcher(serializer serializer.AgentV1Serializer, hostname, agentName string, maxCapacity int) {
	batcherInit.Do(func() {
		batcherInstance = newAsynchronousBatcher(serializer, hostname, agentName, maxCapacity)
	})
}

func newAsynchronousBatcher(serializer serializer.AgentV1Serializer, hostname, agentName string, maxCapacity int) AsynchronousBatcher {
	batcher := AsynchronousBatcher{
		builder:    NewBatchBuilder(maxCapacity),
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
	builder             BatchBuilder
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

type submitHealthCheckData struct {
	checkID check.ID
	stream  health.Stream
	data    health.CheckData
}

type submitHealthStartSnapshot struct {
	checkID         check.ID
	stream          health.Stream
	intervalSeconds int
	expirySeconds   int
}

type submitHealthStopSnapshot struct {
	checkID check.ID
	stream  health.Stream
}

type submitComplete struct {
	checkID check.ID
}

type submitShutdown struct{}

func (batcher *AsynchronousBatcher) sendState(states CheckInstanceBatchStates) {
	if states != nil {

		// Create the topologies
		topologies := make([]topology.Topology, 0)
		for _, state := range states {
			if state.Topology != nil {
				topologies = append(topologies, *state.Topology)
			}
		}

		// Create the healthData payload
		healthData := make([]health.Health, 0)
		for _, state := range states {
			for _, healthRecord := range state.Health {
				healthData = append(healthData, healthRecord)
			}
		}

		payload := map[string]interface{}{
			"internalHostname": batcher.hostname,
			"topologies":       topologies,
			"health":           healthData,
		}

		if err := batcher.serializer.SendJSONToV1Intake(payload); err != nil {
			_ = log.Errorf("error in SendJSONToV1Intake: %s", err)
		}
	}
}

func (batcher *AsynchronousBatcher) run() {
	for {
		s := <-batcher.input
		switch submission := s.(type) {
		case submitComponent:
			batcher.sendState(batcher.builder.AddComponent(submission.checkID, submission.instance, submission.component))
		case submitRelation:
			batcher.sendState(batcher.builder.AddRelation(submission.checkID, submission.instance, submission.relation))
		case submitStartSnapshot:
			batcher.sendState(batcher.builder.StartSnapshot(submission.checkID, submission.instance))
		case submitStopSnapshot:
			batcher.sendState(batcher.builder.StopSnapshot(submission.checkID, submission.instance))

		case submitHealthCheckData:
			batcher.sendState(batcher.builder.AddHealthCheckData(submission.checkID, submission.stream, submission.data))
		case submitHealthStartSnapshot:
			batcher.sendState(batcher.builder.HealthStartSnapshot(submission.checkID, submission.stream, submission.intervalSeconds, submission.expirySeconds))
		case submitHealthStopSnapshot:
			batcher.sendState(batcher.builder.HealthStopSnapshot(submission.checkID, submission.stream))

		case submitComplete:
			batcher.sendState(batcher.builder.FlushIfDataProduced(submission.checkID))
		case submitShutdown:
			return
		default:
			panic(fmt.Sprint("Unknown submission type"))
		}
	}
}

// SubmitComponent submits a component to the batch
func (batcher AsynchronousBatcher) SubmitComponent(checkID check.ID, instance topology.Instance, component topology.Component) {
	log.Debugf("Submitting component for check [%s] instance [%s]: %s", checkID, instance.GoString(), component.JSONString())
	batcher.input <- submitComponent{
		checkID:   checkID,
		instance:  instance,
		component: component,
	}
}

// SubmitRelation submits a relation to the batch
func (batcher AsynchronousBatcher) SubmitRelation(checkID check.ID, instance topology.Instance, relation topology.Relation) {
	log.Debugf("Submitting relation for check [%s] instance [%s]: %s", checkID, instance.GoString(), relation.JSONString())
	batcher.input <- submitRelation{
		checkID:  checkID,
		instance: instance,
		relation: relation,
	}
}

// SubmitStartSnapshot submits start of a snapshot
func (batcher AsynchronousBatcher) SubmitStartSnapshot(checkID check.ID, instance topology.Instance) {
	log.Debugf("Submitting start snapshot for check [%s] instance [%s]", checkID, instance.GoString())
	batcher.input <- submitStartSnapshot{
		checkID:  checkID,
		instance: instance,
	}
}

// SubmitStopSnapshot submits a stop of a snapshot. This always causes a flush of the data downstream
func (batcher AsynchronousBatcher) SubmitStopSnapshot(checkID check.ID, instance topology.Instance) {
	log.Debugf("Submitting stop snapshot for check [%s] instance [%s]", checkID, instance.GoString())
	batcher.input <- submitStopSnapshot{
		checkID:  checkID,
		instance: instance,
	}
}

// SubmitHealthCheckData submits a Health check data record to the batch
func (batcher AsynchronousBatcher) SubmitHealthCheckData(checkID check.ID, stream health.Stream, data health.CheckData) {
	log.Debugf("Submitting Health check data for check [%s] stream [%s]: %s", checkID, stream.GoString(), data.JSONString())
	batcher.input <- submitHealthCheckData{
		checkID: checkID,
		stream:  stream,
		data:    data,
	}
}

// SubmitHealthStartSnapshot submits start of a Health snapshot
func (batcher AsynchronousBatcher) SubmitHealthStartSnapshot(checkID check.ID, stream health.Stream, intervalSeconds int, expirySeconds int) {
	log.Debugf("Submitting start of Health snapshot for check [%s] stream [%s]", checkID, stream.GoString())
	batcher.input <- submitHealthStartSnapshot{
		checkID:         checkID,
		stream:          stream,
		intervalSeconds: intervalSeconds,
		expirySeconds:   expirySeconds,
	}
}

// SubmitHealthStopSnapshot submits a stop of a Health snapshot. This always causes a flush of the data downstream
func (batcher AsynchronousBatcher) SubmitHealthStopSnapshot(checkID check.ID, stream health.Stream) {
	log.Debugf("Submitting stop Health snapshot for check [%s] stream [%s]", checkID, stream.GoString())
	batcher.input <- submitHealthStopSnapshot{
		checkID: checkID,
		stream:  stream,
	}
}

// SubmitComplete signals completion of a check. May trigger a flush only if the check produced data
func (batcher AsynchronousBatcher) SubmitComplete(checkID check.ID) {
	log.Debugf("Submitting complete for check [%s]", checkID)
	batcher.input <- submitComplete{
		checkID: checkID,
	}
}

// Shutdown shuts down the batcher
func (batcher AsynchronousBatcher) Shutdown() {
	batcher.input <- submitShutdown{}
}
