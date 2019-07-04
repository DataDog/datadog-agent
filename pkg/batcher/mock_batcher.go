package batcher

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

// MockBatcher mocks implementation of a batcher
type MockBatcher struct {
	CollectedTopology TopologyBuilder
}

func createMockBatcher() MockBatcher {
	return MockBatcher{
		CollectedTopology: NewTopologyBuilder(1000),
	}
}

// SubmitComponent mock
func (batcher MockBatcher) SubmitComponent(checkID check.ID, instance topology.Instance, component topology.Component) {
	batcher.CollectedTopology.AddComponent(checkID, instance, component)
}

// SubmitRelation mock
func (batcher MockBatcher) SubmitRelation(checkID check.ID, instance topology.Instance, relation topology.Relation) {
	batcher.CollectedTopology.AddRelation(checkID, instance, relation)
}

// SubmitStartSnapshot mock
func (batcher MockBatcher) SubmitStartSnapshot(checkID check.ID, instance topology.Instance) {
	batcher.CollectedTopology.StartSnapshot(checkID, instance)
}

// SubmitStopSnapshot mock
func (batcher MockBatcher) SubmitStopSnapshot(checkID check.ID, instance topology.Instance) {
	batcher.CollectedTopology.StopSnapshot(checkID, instance)
}

// SubmitComplete mock
func (batcher MockBatcher) SubmitComplete(checkID check.ID) {

}

// Shutdown mock
func (batcher MockBatcher) Shutdown() {}
