package batcher

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

// Topologies is the type representing topologies gathered per check
type Topologies map[check.ID]topology.Topology

// TopologyBuilder is a helper class to build topology based on submitted data
type TopologyBuilder struct {
	topologies Topologies
	// Count the amount of elements we gathered
	elementCount int
	// Amount of elements when we flush
	flushElementCount int
}

// NewTopologyBuilder constructs a TopologyBuilder
func NewTopologyBuilder(flushElementCount int) TopologyBuilder {
	return TopologyBuilder{
		topologies:        make(map[check.ID]topology.Topology),
		elementCount:      0,
		flushElementCount: flushElementCount,
	}
}

func (builder *TopologyBuilder) getTopology(checkID check.ID, instance topology.Instance) topology.Topology {
	if value, ok := builder.topologies[checkID]; ok {
		return value
	}

	topology := topology.Topology{
		StartSnapshot: false,
		StopSnapshot:  false,
		Instance:      instance,
		Components:    make([]topology.Component, 0),
		Relations:     make([]topology.Relation, 0),
	}
	builder.topologies[checkID] = topology
	return topology
}

// AddComponent adds a component
func (builder *TopologyBuilder) AddComponent(checkID check.ID, instance topology.Instance, component topology.Component) Topologies {
	topology := builder.getTopology(checkID, instance)
	topology.Components = append(topology.Components, component)
	builder.topologies[checkID] = topology
	return builder.incrementAndTryFlush()
}

// AddRelation adds a relation
func (builder *TopologyBuilder) AddRelation(checkID check.ID, instance topology.Instance, relation topology.Relation) Topologies {
	topology := builder.getTopology(checkID, instance)
	topology.Relations = append(topology.Relations, relation)
	builder.topologies[checkID] = topology
	return builder.incrementAndTryFlush()
}

// StartSnapshot starts a snapshot
func (builder *TopologyBuilder) StartSnapshot(checkID check.ID, instance topology.Instance) Topologies {
	topology := builder.getTopology(checkID, instance)
	topology.StartSnapshot = true
	builder.topologies[checkID] = topology
	return nil
}

// StopSnapshot stops a snapshot. This will always flush
func (builder *TopologyBuilder) StopSnapshot(checkID check.ID, instance topology.Instance) Topologies {
	topology := builder.getTopology(checkID, instance)
	topology.StopSnapshot = true
	builder.topologies[checkID] = topology
	// We always flush after a StopSnapshot to limit latency
	return builder.Flush()
}

// Flush the collected data. Returning the data and wiping the current build up topology
func (builder *TopologyBuilder) Flush() Topologies {
	data := builder.topologies
	builder.topologies = make(map[check.ID]topology.Topology)
	builder.elementCount = 0
	return data
}

func (builder *TopologyBuilder) incrementAndTryFlush() Topologies {
	builder.elementCount = builder.elementCount + 1

	if builder.elementCount >= builder.flushElementCount {
		return builder.Flush()
	}

	return nil
}

// FlushIfDataProduced checks whether the check produced data, if so, flush
func (builder *TopologyBuilder) FlushIfDataProduced(checkID check.ID) Topologies {
	if _, ok := builder.topologies[checkID]; ok {
		return builder.Flush()
	}

	return nil
}
