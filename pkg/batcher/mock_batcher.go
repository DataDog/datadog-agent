package batcher

import "github.com/StackVista/stackstate-agent/pkg/topology"

type MockBatcher struct {
	CollectedTopology TopologyBuilder
}

func createMockBatcher() MockBatcher {
	return MockBatcher{
		CollectedTopology: NewTopologyBuilder(),
	}
}

func (batcher MockBatcher) SubmitComponent(checkID string,  instance topology.Instance, component topology.Component) {
	batcher.CollectedTopology.AddComponent(checkID, instance, component)
}

func (batcher MockBatcher) SubmitRelation(checkID string,  instance topology.Instance, relation topology.Relation) {
	batcher.CollectedTopology.AddRelation(checkID, instance, relation)
}

func (batcher MockBatcher) SubmitStartSnapshot(checkID string, instance topology.Instance) {
	batcher.CollectedTopology.StartSnapshot(checkID, instance)
}

func (batcher MockBatcher) SubmitStopSnapshot(checkID string, instance topology.Instance) {
	batcher.CollectedTopology.StopSnapshot(checkID, instance)
}
