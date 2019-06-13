package batcher

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

type MockBatcher struct {
	CollectedTopology TopologyBuilder
}

func createMockBatcher() MockBatcher {
	return MockBatcher{
		CollectedTopology: NewTopologyBuilder(1000),
	}
}

func (batcher MockBatcher) SubmitComponent(checkID check.ID, instance topology.Instance, component topology.Component) {
	batcher.CollectedTopology.AddComponent(checkID, instance, component)
}

func (batcher MockBatcher) SubmitRelation(checkID check.ID, instance topology.Instance, relation topology.Relation) {
	batcher.CollectedTopology.AddRelation(checkID, instance, relation)
}

func (batcher MockBatcher) SubmitStartSnapshot(checkID check.ID, instance topology.Instance) {
	batcher.CollectedTopology.StartSnapshot(checkID, instance)
}

func (batcher MockBatcher) SubmitStopSnapshot(checkID check.ID, instance topology.Instance) {
	batcher.CollectedTopology.StopSnapshot(checkID, instance)
}

func (batcher MockBatcher) SubmitComplete(checkID check.ID) {

}

func (batcher MockBatcher) Shutdown() {}
