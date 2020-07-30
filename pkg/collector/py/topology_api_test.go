package py

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	collectorutil "github.com/StackVista/stackstate-agent/pkg/collector/util"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestComponentTopology(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestComponentCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	expectedTopology := mockBatcher.CollectedTopology.Flush()
	components, relations := getAgentIntegrationTopology(t, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance: topology.Instance{
				Type: "type",
				URL:  "url",
			},
			Components: append(components, []topology.Component{
				{
					ExternalID: "myid",
					Type:       topology.Type{Name: "mytype"},
					Data: topology.Data{
						"nestedobject": map[string]interface{}{"nestedkey": "nestedValue"},
						"key":          "value",
						"intlist":      []interface{}{int64(1)}, "emptykey": map[string]interface{}{},
					},
				},
			}...),
			Relations: relations,
		},
	}), expectedTopology)
}

func TestRelationTopology(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestRelationCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	expectedTopology := mockBatcher.CollectedTopology.Flush()
	components, relations := getAgentIntegrationTopology(t, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      topology.Instance{Type: "type", URL: "url"},
			Components:    components,
			Relations: append(relations, []topology.Relation{
				{
					ExternalID: "source-mytype-target",
					SourceID:   "source",
					TargetID:   "target",
					Type:       topology.Type{Name: "mytype"},
					Data: map[string]interface{}{
						"nestedobject": map[string]interface{}{"nestedkey": "nestedValue"},
						"key":          "value",
						"intlist":      []interface{}{int64(1)},
						"emptykey":     map[string]interface{}{},
					},
				},
			}...),
		},
	}), expectedTopology)
}

func TestStartSnapshotCheck(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestStartSnapshotCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	expectedTopology := mockBatcher.CollectedTopology.Flush()
	components, relations := getAgentIntegrationTopology(t, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      topology.Instance{Type: "type", URL: "url"},
			Components:    components,
			Relations:     relations,
		},
	}), expectedTopology)
}

func TestStopSnapshotCheck(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestStopSnapshotCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	expectedTopology := mockBatcher.CollectedTopology.Flush()
	components, relations := getAgentIntegrationTopology(t, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      topology.Instance{Type: "type", URL: "url"},
			Components:    components,
			Relations:     relations,
		},
	}), expectedTopology)
}

func getAgentIntegrationTopology(t *testing.T, expectedTopology batcher.Topologies) ([]topology.Component, []topology.Relation) {
	var integrationIdentifier, integrationInstanceIdentifier string
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[1].Data["events"].([]interface{}) {
		integrationIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[2].Data["events"].([]interface{}) {
		integrationInstanceIdentifier = e.(map[string]interface{})["identifier"].(string)
	}

	pid := os.Getpid()
	ct, err := collectorutil.GetProcessCreateTime(int32(pid))
	if err != nil {
		t.Fatal(err)
	}
	host, err := util.GetHostname()
	if err != nil {
		t.Fatal(err)
	}

	return []topology.Component{
			{
				ExternalID: fmt.Sprintf("urn:stackstate-agent:/%s", host),
				Type:       topology.Type{Name: "stackstate-agent"},
				Data: topology.Data{
					"hostname": host,
					"identifiers": []interface{}{
						fmt.Sprintf("urn:process:/%s:%d:%d", host, pid, ct),
					},
					"name": fmt.Sprintf("StackState Agent:%s", host),
					"tags": []interface{}{fmt.Sprintf("hostname:%s", host), "stackstate-agent"},
				},
			},
			{
				ExternalID: fmt.Sprintf("urn:agent-integration:/%s:type", host),
				Type:       topology.Type{Name: "agent-integration"},
				Data: topology.Data{
					"tags":        []interface{}{fmt.Sprintf("hostname:%s", host), "agent-integration:type"},
					"hostname":    host,
					"integration": "type",
					"checks": []interface{}{
						map[string]interface{}{
							"stream_id": int64(-1), "name": "Integration Health", "is_service_check_health_check": int64(1),
						},
					},
					"events": []interface{}{map[string]interface{}{
						"stream_id":  int64(-1),
						"identifier": integrationIdentifier,
						"conditions": []interface{}{
							map[string]interface{}{"value": host, "key": "hostname"},
							map[string]interface{}{"value": "type", "key": "integration-type"},
						},
						"name": "Service Checks",
					}},
					"name": fmt.Sprintf("%s:type", host),
				},
			},
			{
				ExternalID: fmt.Sprintf("urn:agent-integration-instance:/%s:type:url", host),
				Type:       topology.Type{Name: "agent-integration-instance"},
				Data: topology.Data{
					"name": "type:url",
					"tags": []interface{}{
						fmt.Sprintf("hostname:%s", host), "agent-integration:type", "agent-integration-url:url",
					},
					"hostname":    host,
					"integration": "type",
					"checks": []interface{}{
						map[string]interface{}{
							"stream_id":                     int64(-1),
							"name":                          "Integration Instance Health",
							"is_service_check_health_check": int64(1),
						},
					},
					"events": []interface{}{
						map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{"value": host, "key": "hostname"},
								map[string]interface{}{"value": "type", "key": "integration-type"},
								map[string]interface{}{"value": "url", "key": "integration-url"},
							},
							"name":       "Service Checks",
							"stream_id":  int64(-1),
							"identifier": integrationInstanceIdentifier,
						},
					},
				},
			},
		},
		[]topology.Relation{
			{
				ExternalID: fmt.Sprintf("urn:stackstate-agent:/%s-runs-urn:agent-integration:/%s:type", host, host),
				SourceID:   fmt.Sprintf("urn:stackstate-agent:/%s", host),
				TargetID:   fmt.Sprintf("urn:agent-integration:/%s:type", host),
				Type:       topology.Type{Name: "runs"}, Data: map[string]interface{}{},
			}, {
				ExternalID: fmt.Sprintf("urn:agent-integration:/%s:type-has-urn:agent-integration-instance:/%s:type:url", host, host),
				SourceID:   fmt.Sprintf("urn:agent-integration:/%s:type", host),
				TargetID:   fmt.Sprintf("urn:agent-integration-instance:/%s:type:url", host),
				Type:       topology.Type{Name: "has"},
				Data:       map[string]interface{}{},
			},
		}
}
