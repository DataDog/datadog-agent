package testdata

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/kubeapi"
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

// NewLogTopologySubmitter creates a new instance of TestTopologySubmitter
func NewLogTopologySubmitter(checkID check.ID, instance topology.Instance) kubeapi.TopologySubmitter {
	return &LogTopologySubmitter{
		CheckID:  checkID,
		Instance: instance,
	}
}

// TestTopologySubmitter provides functionality to submit topology data with the Batcher.
type LogTopologySubmitter struct {
	CheckID  check.ID
	Instance topology.Instance
}

func (lts *LogTopologySubmitter) SubmitStartSnapshot() {}
func (lts *LogTopologySubmitter) SubmitStopSnapshot()  {}
func (lts *LogTopologySubmitter) SubmitComplete()      {}

// SubmitRelation takes a component and submits it with the Batcher
func (lts *LogTopologySubmitter) SubmitComponent(component *topology.Component) {
	fmt.Printf("Submitting Component: %s\n", component.ExternalID)
}

// SubmitRelation takes a relation and submits it with the Batcher
func (lts *LogTopologySubmitter) SubmitRelation(relation *topology.Relation) {
	fmt.Printf("Submitting Relation: %s\n", relation.ExternalID)
}

// HandleError handles any errors during topology gathering
func (lts *LogTopologySubmitter) HandleError(err error) {
	_ = fmt.Errorf("Handling Error: %s\n", err.Error())
}
