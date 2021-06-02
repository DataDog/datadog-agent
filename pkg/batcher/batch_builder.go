package batcher

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/health"
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

// CheckInstanceBatchState is the type representing batched data per check instance
type CheckInstanceBatchState struct {
	Topology *topology.Topology
	Health   map[string]health.Health
}

// CheckInstanceBatchStates is the type representing batched data for all check instances
type CheckInstanceBatchStates map[check.ID]CheckInstanceBatchState

// BatchBuilder is a helper class to build Topology based on submitted data, this data structure is not thread safe
type BatchBuilder struct {
	states CheckInstanceBatchStates
	// Count the amount of elements we gathered
	elementCount int
	// Amount of elements when we flush
	maxCapacity int
}

// NewBatchBuilder constructs a BatchBuilder
func NewBatchBuilder(maxCapacity int) BatchBuilder {
	return BatchBuilder{
		states:       make(map[check.ID]CheckInstanceBatchState),
		elementCount: 0,
		maxCapacity:  maxCapacity,
	}
}

func (builder *BatchBuilder) getOrCreateState(checkID check.ID) CheckInstanceBatchState {
	if value, ok := builder.states[checkID]; ok {
		return value
	}

	state := CheckInstanceBatchState{
		Topology: nil,
		Health:   make(map[string]health.Health),
	}
	builder.states[checkID] = state
	return state
}

func (builder *BatchBuilder) getOrCreateTopology(checkID check.ID, instance topology.Instance) *topology.Topology {
	state := builder.getOrCreateState(checkID)

	if state.Topology != nil {
		return state.Topology
	}

	builder.states[checkID] = CheckInstanceBatchState{
		Topology: &topology.Topology{
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      instance,
			Components:    make([]topology.Component, 0),
			Relations:     make([]topology.Relation, 0),
		},
		Health: state.Health,
	}
	return builder.states[checkID].Topology
}

func (builder *BatchBuilder) getOrCreateHealth(checkID check.ID, stream health.Stream) health.Health {
	state := builder.getOrCreateState(checkID)

	if value, ok := state.Health[stream.GoString()]; ok {
		return value
	}

	builder.states[checkID].Health[stream.GoString()] = health.Health{
		StartSnapshot: nil,
		StopSnapshot:  nil,
		Stream:        stream,
		CheckStates:   make([]health.CheckData, 0),
	}

	return builder.states[checkID].Health[stream.GoString()]
}

// AddComponent adds a component
func (builder *BatchBuilder) AddComponent(checkID check.ID, instance topology.Instance, component topology.Component) CheckInstanceBatchStates {
	topologyData := builder.getOrCreateTopology(checkID, instance)
	topologyData.Components = append(topologyData.Components, component)
	return builder.incrementAndTryFlush()
}

// AddRelation adds a relation
func (builder *BatchBuilder) AddRelation(checkID check.ID, instance topology.Instance, relation topology.Relation) CheckInstanceBatchStates {
	topologyData := builder.getOrCreateTopology(checkID, instance)
	topologyData.Relations = append(topologyData.Relations, relation)
	return builder.incrementAndTryFlush()
}

// TopologyStartSnapshot starts a snapshot
func (builder *BatchBuilder) TopologyStartSnapshot(checkID check.ID, instance topology.Instance) CheckInstanceBatchStates {
	topologyData := builder.getOrCreateTopology(checkID, instance)
	topologyData.StartSnapshot = true
	return nil
}

// TopologyStopSnapshot stops a snapshot. This will always flush
func (builder *BatchBuilder) TopologyStopSnapshot(checkID check.ID, instance topology.Instance) CheckInstanceBatchStates {
	topologyData := builder.getOrCreateTopology(checkID, instance)
	topologyData.StopSnapshot = true
	// We always flush after a TopologyStopSnapshot to limit latency
	return builder.Flush()
}

// AddHealthCheckData adds a component
func (builder *BatchBuilder) AddHealthCheckData(checkID check.ID, stream health.Stream, data health.CheckData) CheckInstanceBatchStates {
	healthData := builder.getOrCreateHealth(checkID, stream)
	healthData.CheckStates = append(healthData.CheckStates, data)
	builder.states[checkID].Health[stream.GoString()] = healthData
	return builder.incrementAndTryFlush()
}

// HealthStartSnapshot starts a Health snapshot
func (builder *BatchBuilder) HealthStartSnapshot(checkID check.ID, stream health.Stream, repeatIntervalSeconds int, expirySeconds int) CheckInstanceBatchStates {
	healthData := builder.getOrCreateHealth(checkID, stream)
	healthData.StartSnapshot = &health.StartSnapshotMetadata{
		RepeatIntervalS: repeatIntervalSeconds,
		ExpiryIntervalS: expirySeconds,
	}
	builder.states[checkID].Health[stream.GoString()] = healthData
	return nil
}

// HealthStopSnapshot stops a Health snapshot. This will always flush
func (builder *BatchBuilder) HealthStopSnapshot(checkID check.ID, stream health.Stream) CheckInstanceBatchStates {
	healthData := builder.getOrCreateHealth(checkID, stream)
	healthData.StopSnapshot = &health.StopSnapshotMetadata{}
	builder.states[checkID].Health[stream.GoString()] = healthData
	// We always flush after a TopologyStopSnapshot to limit latency
	return builder.Flush()
}

// Flush the collected data. Returning the data and wiping the current build up Topology
func (builder *BatchBuilder) Flush() CheckInstanceBatchStates {
	data := builder.states
	builder.states = make(map[check.ID]CheckInstanceBatchState)
	builder.elementCount = 0
	return data
}

func (builder *BatchBuilder) incrementAndTryFlush() CheckInstanceBatchStates {
	builder.elementCount = builder.elementCount + 1

	if builder.elementCount >= builder.maxCapacity {
		return builder.Flush()
	}

	return nil
}

// FlushIfDataProduced checks whether the check produced data, if so, flush
func (builder *BatchBuilder) FlushIfDataProduced(checkID check.ID) CheckInstanceBatchStates {
	if _, ok := builder.states[checkID]; ok {
		return builder.Flush()
	}

	return nil
}
