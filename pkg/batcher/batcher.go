package batcher

import (
	"github.com/StackVista/stackstate-agent/pkg/serializer"
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

var (
	batcherInstance Batcher
)

type Batcher interface {
	SubmitComponent(checkID string, instance topology.Instance, component topology.Component)
	SubmitRelation(checkID string, instance topology.Instance, relation topology.Relation)
	SubmitStartSnapshot(checkID string, instance topology.Instance)
	SubmitStopSnapshot(checkID string, instance topology.Instance)
}

func InitBatcher(s serializer.MetricSerializer, hostname, agentName string) {
	// [BS] TODO: implement real batcher
}

func GetBatcher() Batcher {
	return batcherInstance
}
func NewMockBatcher() MockBatcher {
	batcher := createMockBatcher()
	batcherInstance = batcher
	return batcher
}



