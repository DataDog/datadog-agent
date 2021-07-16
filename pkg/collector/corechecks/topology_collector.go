package corechecks

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
)

// CheckTopologyCollector contains all the metadata needed to produce disk topology
type CheckTopologyCollector struct {
	CheckID          check.ID
	TopologyInstance topology.Instance
}

// MakeCheckProcessTopologyCollector returns an instance of the CheckTopologyCollector
func MakeCheckProcessTopologyCollector(checkID check.ID) CheckTopologyCollector {
	return CheckTopologyCollector{
		CheckID: checkID,
		TopologyInstance: topology.Instance{
			Type: "process",
			URL:  "agents",
		},
	}
}

// MakeCheckTopologyCollector returns an instance of the CheckTopologyCollector
func MakeCheckTopologyCollector(checkID check.ID, instance topology.Instance) CheckTopologyCollector {
	return CheckTopologyCollector{
		CheckID:          checkID,
		TopologyInstance: instance,
	}
}
