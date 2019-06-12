package batcher

import "github.com/StackVista/stackstate-agent/pkg/topology"

type TopologyBuilder struct {
	topologies map[string]topology.Topology
}

func NewTopologyBuilder() TopologyBuilder{
	return TopologyBuilder{
		topologies: make(map[string]topology.Topology),
	}
}

func (builder *TopologyBuilder) getTopology(checkID string, instance topology.Instance) topology.Topology {
	if value, ok := builder.topologies[checkID]; ok {
		return value
	} else {
		topology := topology.Topology{
			StartSnapshot: false,
			StopSnapshot: false,
			Instance: instance,
			Components: make([]topology.Component, 0),
			Relations: make([]topology.Relation, 0),
		}
		builder.topologies[checkID] = topology
		return topology
	}
}

func (builder *TopologyBuilder) AddComponent(checkID string, instance topology.Instance, component topology.Component) {
	topology := builder.getTopology(checkID, instance)
	topology.Components = append(topology.Components, component)
	builder.topologies[checkID] = topology
}

func (builder *TopologyBuilder) AddRelation(checkID string, instance topology.Instance, relation topology.Relation) {
	topology := builder.getTopology(checkID, instance)
	topology.Relations = append(topology.Relations, relation)
	builder.topologies[checkID] = topology
}

func (builder *TopologyBuilder) StartSnapshot(checkID string, instance topology.Instance) {
	topology := builder.getTopology(checkID, instance)
	topology.StartSnapshot = true
	builder.topologies[checkID] = topology
}

func (builder *TopologyBuilder) StopSnapshot(checkID string, instance topology.Instance) {
	topology := builder.getTopology(checkID, instance)
	topology.StopSnapshot = true
	builder.topologies[checkID] = topology
}

func (builder *TopologyBuilder) Flush() map[string] topology.Topology {
	data := builder.topologies
	builder.topologies = make(map[string]topology.Topology)
	return data
}
