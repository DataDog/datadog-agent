package corechecks

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMakeCheckTopologyCollector(t *testing.T) {
	checkID := check.ID("process_check_topology")
	instance := topology.Instance{
		Type: "test",
		URL:  "url",
	}
	ptc := MakeCheckTopologyCollector(checkID, instance)
	assert.Equal(t, checkID, ptc.CheckID)
	assert.Equal(t, instance, ptc.TopologyInstance)
}

func TestMakeCheckProcessTopologyCollector(t *testing.T) {
	checkID := check.ID("process_check_topology")
	ptc := MakeCheckProcessTopologyCollector(checkID)
	assert.Equal(t, checkID, ptc.CheckID)
	expectedInstance := topology.Instance{
		Type: "process",
		URL:  "agents",
	}
	assert.Equal(t, expectedInstance, ptc.TopologyInstance)
}
