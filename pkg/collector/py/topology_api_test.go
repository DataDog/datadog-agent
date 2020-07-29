package py

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/collector/util"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestComponentTopology(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestComponentCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	pid := os.Getpid()
	ct, err := util.GetProcessCreateTime(int32(pid))
	if err != nil {
		t.Fatal(err)
	}

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	var integrationIdentifier, integrationInstanceIdentifier string
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[1].Data["events"].([]interface{}) {
		integrationIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[2].Data["events"].([]interface{}) {
		integrationInstanceIdentifier = e.(map[string]interface{})["identifier"].(string)
	}

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance: topology.Instance{
				Type: "type",
				URL:  "url",
			},
			Components: []topology.Component{
				{
					ExternalID: "urn:stackstate-agent/:zandre-XPS-15-9570",
					Type:       topology.Type{Name: "stackstate-agent"},
					Data: topology.Data{
						"hostname": "zandre-XPS-15-9570",
						"identifiers": []interface{}{
							fmt.Sprintf("urn:process/:zandre-XPS-15-9570:%d:%d", pid, ct),
						},
						"name": "StackState Agent:zandre-XPS-15-9570",
						"tags": []interface{}{"hostname:zandre-XPS-15-9570", "stackstate-agent"},
					},
				},
				{
					ExternalID: "urn:agent-integration/:zandre-XPS-15-9570:type",
					Type:       topology.Type{Name: "agent-integration"},
					Data: topology.Data{
						"tags":        []interface{}{"hostname:zandre-XPS-15-9570", "agent-integration:type"},
						"hostname":    "zandre-XPS-15-9570",
						"integration": "type",
						"checks": []interface{}{
							map[string]interface{}{
								"stream_id": int64(-1), "name": "Integration Health", "is_service_check_health_check":int64(1),
							},
						},
						"events": []interface{}{map[string]interface{}{
							"stream_id": int64(-1),
							"identifier": integrationIdentifier,
							"conditions": []interface{}{
								map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
								map[string]interface{}{"value": "type", "key": "integration-type"},
							},
							"name": "Service Checks",
						}},
						"name": "zandre-XPS-15-9570:type",
					},
				},
				{
					ExternalID: "urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					Type:       topology.Type{Name: "agent-integration-instance"},
					Data: topology.Data{
						"name": "type:url",
						"tags": []interface{}{
							"hostname:zandre-XPS-15-9570", "agent-integration:type", "agent-integration-url:url",
						},
						"hostname":    "zandre-XPS-15-9570",
						"integration": "type",
						"checks": []interface{}{
							map[string]interface{}{
								"stream_id":                     int64(-1),
								"name":                          "Integration Instance Health",
								"is_service_check_health_check":int64(1),
							},
						},
						"events": []interface{}{
							map[string]interface{}{
								"conditions": []interface{}{
									map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
									map[string]interface{}{"value": "type", "key": "integration-type"},
									map[string]interface{}{"value": "url", "key": "integration-url"},
								},
								"name":       "Service Checks",
								"stream_id":  int64(-1),
								"identifier": integrationInstanceIdentifier,
							},
						},
					},
				}, {
					ExternalID: "myid",
					Type:       topology.Type{Name: "mytype"},
					Data: topology.Data{
						"nestedobject": map[string]interface{}{"nestedkey": "nestedValue"},
						"key":          "value",
						"intlist":      []interface{}{int64(1)}, "emptykey": map[string]interface{}{},
					},
				}},
			Relations: []topology.Relation{
				{
					ExternalID: "urn:stackstate-agent/:zandre-XPS-15-9570-runs-urn:agent-integration/:zandre-XPS-15-9570:type",
					SourceID:   "urn:stackstate-agent/:zandre-XPS-15-9570",
					TargetID:   "urn:agent-integration/:zandre-XPS-15-9570:type",
					Type:       topology.Type{Name: "runs"}, Data: map[string]interface{}{},
				}, {
					ExternalID: "urn:agent-integration/:zandre-XPS-15-9570:type-has-urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					SourceID:   "urn:agent-integration/:zandre-XPS-15-9570:type",
					TargetID:   "urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					Type:       topology.Type{Name: "has"},
					Data:       map[string]interface{}{},
				},
			},
		},
	}), expectedTopology)
}

func TestRelationTopology(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestRelationCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	pid := os.Getpid()
	ct, err := util.GetProcessCreateTime(int32(pid))
	if err != nil {
		t.Fatal(err)
	}

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	var integrationIdentifier, integrationInstanceIdentifier string
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[1].Data["events"].([]interface{}) {
		integrationIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[2].Data["events"].([]interface{}) {
		integrationInstanceIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      topology.Instance{Type: "type", URL: "url"},
			Components: []topology.Component{
				{
					ExternalID: "urn:stackstate-agent/:zandre-XPS-15-9570",
					Type:       topology.Type{Name: "stackstate-agent"},
					Data: topology.Data{
						"hostname": "zandre-XPS-15-9570",
						"identifiers": []interface{}{
							fmt.Sprintf("urn:process/:zandre-XPS-15-9570:%d:%d", pid, ct),
						},
						"name": "StackState Agent:zandre-XPS-15-9570",
						"tags": []interface{}{"hostname:zandre-XPS-15-9570", "stackstate-agent"},
					},
				}, {
					ExternalID: "urn:agent-integration/:zandre-XPS-15-9570:type",
					Type:       topology.Type{Name: "agent-integration"},
					Data: topology.Data{
						"checks": []interface{}{
							map[string]interface{}{"stream_id": int64(-1), "name": "Integration Health", "is_service_check_health_check":int64(1)},
						},
						"events": []interface{}{
							map[string]interface{}{
								"conditions": []interface{}{
									map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
									map[string]interface{}{"value": "type", "key": "integration-type"},
								},
								"name":       "Service Checks",
								"stream_id":  int64(-1),
								"identifier": integrationIdentifier,
							},
						},
						"name":     "zandre-XPS-15-9570:type",
						"tags":     []interface{}{"hostname:zandre-XPS-15-9570", "agent-integration:type"},
						"hostname": "zandre-XPS-15-9570", "integration": "type"},
				}, {
					ExternalID: "urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					Type:       topology.Type{Name: "agent-integration-instance"},
					Data: topology.Data{
						"name":        "type:url",
						"tags":        []interface{}{"hostname:zandre-XPS-15-9570", "agent-integration:type", "agent-integration-url:url"},
						"hostname":    "zandre-XPS-15-9570",
						"integration": "type",
						"checks": []interface{}{
							map[string]interface{}{
								"stream_id":                     int64(-1),
								"name":                          "Integration Instance Health",
								"is_service_check_health_check":int64(1),
							},
						},
						"events": []interface{}{
							map[string]interface{}{
								"conditions": []interface{}{
									map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
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
			Relations: []topology.Relation{
				{
					ExternalID: "urn:stackstate-agent/:zandre-XPS-15-9570-runs-urn:agent-integration/:zandre-XPS-15-9570:type",
					SourceID:   "urn:stackstate-agent/:zandre-XPS-15-9570",
					TargetID:   "urn:agent-integration/:zandre-XPS-15-9570:type",
					Type:       topology.Type{Name: "runs"},
					Data:       map[string]interface{}{},
				}, {
					ExternalID: "urn:agent-integration/:zandre-XPS-15-9570:type-has-urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					SourceID:   "urn:agent-integration/:zandre-XPS-15-9570:type",
					TargetID:   "urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					Type:       topology.Type{Name: "has"},
					Data:       map[string]interface{}{},
				}, {
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
			},
		},
	}), expectedTopology)
}

func TestStartSnapshotCheck(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestStartSnapshotCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	pid := os.Getpid()
	ct, err := util.GetProcessCreateTime(int32(pid))
	if err != nil {
		t.Fatal(err)
	}

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	var integrationIdentifier, integrationInstanceIdentifier string
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[1].Data["events"].([]interface{}) {
		integrationIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[2].Data["events"].([]interface{}) {
		integrationInstanceIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      topology.Instance{Type: "type", URL: "url"},
			Components: []topology.Component{
				{
					ExternalID: "urn:stackstate-agent/:zandre-XPS-15-9570",
					Type:       topology.Type{Name: "stackstate-agent"},
					Data: topology.Data{
						"tags":     []interface{}{"hostname:zandre-XPS-15-9570", "stackstate-agent"},
						"hostname": "zandre-XPS-15-9570",
						"identifiers": []interface{}{
							fmt.Sprintf("urn:process/:zandre-XPS-15-9570:%d:%d", pid, ct),
						},
						"name": "StackState Agent:zandre-XPS-15-9570",
					},
				},
				{
					ExternalID: "urn:agent-integration/:zandre-XPS-15-9570:type",
					Type:       topology.Type{Name: "agent-integration"},
					Data: topology.Data{
						"hostname":    "zandre-XPS-15-9570",
						"integration": "type",
						"checks": []interface{}{
							map[string]interface{}{"name": "Integration Health", "is_service_check_health_check":int64(1), "stream_id": int64(-1)},
						},
						"events": []interface{}{
							map[string]interface{}{
								"stream_id":  int64(-1),
								"identifier": integrationIdentifier,
								"conditions": []interface{}{
									map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
									map[string]interface{}{"value": "type", "key": "integration-type"},
								},
								"name": "Service Checks",
							},
						},
						"name": "zandre-XPS-15-9570:type",
						"tags": []interface{}{"hostname:zandre-XPS-15-9570", "agent-integration:type"},
					},
				}, {
					ExternalID: "urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					Type:       topology.Type{Name: "agent-integration-instance"},
					Data: topology.Data{
						"checks": []interface{}{map[string]interface{}{
							"stream_id":                     int64(-1),
							"name":                          "Integration Instance Health",
							"is_service_check_health_check": int64(1),
						}},
						"events": []interface{}{
							map[string]interface{}{
								"name":       "Service Checks",
								"stream_id":  int64(-1),
								"identifier": integrationInstanceIdentifier,
								"conditions": []interface{}{
									map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
									map[string]interface{}{"value": "type", "key": "integration-type"},
									map[string]interface{}{"value": "url", "key": "integration-url"},
								},
							},
						},
						"name":        "type:url",
						"tags":        []interface{}{"hostname:zandre-XPS-15-9570", "agent-integration:type", "agent-integration-url:url"},
						"hostname":    "zandre-XPS-15-9570",
						"integration": "type",
					},
				},
			},
			Relations: []topology.Relation{
				{
					ExternalID: "urn:stackstate-agent/:zandre-XPS-15-9570-runs-urn:agent-integration/:zandre-XPS-15-9570:type",
					SourceID:   "urn:stackstate-agent/:zandre-XPS-15-9570",
					TargetID:   "urn:agent-integration/:zandre-XPS-15-9570:type",
					Type:       topology.Type{Name: "runs"},
					Data:       map[string]interface{}{},
				}, {
					ExternalID: "urn:agent-integration/:zandre-XPS-15-9570:type-has-urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					SourceID:   "urn:agent-integration/:zandre-XPS-15-9570:type",
					TargetID:   "urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					Type:       topology.Type{Name: "has"},
					Data:       map[string]interface{}{},
				},
			},
		},
	}), expectedTopology)
}

func TestStopSnapshotCheck(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestStopSnapshotCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	pid := os.Getpid()
	ct, err := util.GetProcessCreateTime(int32(pid))
	if err != nil {
		t.Fatal(err)
	}

	expectedTopology := mockBatcher.CollectedTopology.Flush()
	var integrationIdentifier, integrationInstanceIdentifier string
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[1].Data["events"].([]interface{}) {
		integrationIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	for _, e := range expectedTopology["testtopology:c3d960f8ff8a5c55"].Components[2].Data["events"].([]interface{}) {
		integrationInstanceIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      topology.Instance{Type: "type", URL: "url"},
			Components: []topology.Component{
				{
					ExternalID: "urn:stackstate-agent/:zandre-XPS-15-9570",
					Type:       topology.Type{Name: "stackstate-agent"},
					Data: topology.Data{"hostname": "zandre-XPS-15-9570",
						"identifiers": []interface{}{
							fmt.Sprintf("urn:process/:zandre-XPS-15-9570:%d:%d", pid, ct),
						},
						"name": "StackState Agent:zandre-XPS-15-9570",
						"tags": []interface{}{"hostname:zandre-XPS-15-9570", "stackstate-agent"},
					},
				}, {
					ExternalID: "urn:agent-integration/:zandre-XPS-15-9570:type",
					Type:       topology.Type{Name: "agent-integration"},
					Data: topology.Data{
						"integration": "type",
						"checks": []interface{}{
							map[string]interface{}{"name": "Integration Health", "is_service_check_health_check":int64(1), "stream_id": int64(-1)},
						},
						"events": []interface{}{
							map[string]interface{}{
								"identifier": integrationIdentifier,
								"conditions": []interface{}{
									map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
									map[string]interface{}{"value": "type", "key": "integration-type"},
								},
								"name":      "Service Checks",
								"stream_id": int64(-1),
							},
						},
						"name":     "zandre-XPS-15-9570:type",
						"tags":     []interface{}{"hostname:zandre-XPS-15-9570", "agent-integration:type"},
						"hostname": "zandre-XPS-15-9570",
					},
				}, {
					ExternalID: "urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					Type:       topology.Type{Name: "agent-integration-instance"},
					Data: topology.Data{
						"name":        "type:url",
						"tags":        []interface{}{"hostname:zandre-XPS-15-9570", "agent-integration:type", "agent-integration-url:url"},
						"hostname":    "zandre-XPS-15-9570",
						"integration": "type",
						"checks": []interface{}{
							map[string]interface{}{"stream_id": int64(-1), "name": "Integration Instance Health", "is_service_check_health_check":int64(1)},
						},
						"events": []interface{}{
							map[string]interface{}{
								"stream_id":  int64(-1),
								"identifier": integrationInstanceIdentifier,
								"conditions": []interface{}{
									map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
									map[string]interface{}{"value": "type", "key": "integration-type"},
									map[string]interface{}{"value": "url", "key": "integration-url"},
								},
								"name": "Service Checks",
							},
						},
					},
				},
			},
			Relations: []topology.Relation{
				{
					ExternalID: "urn:stackstate-agent/:zandre-XPS-15-9570-runs-urn:agent-integration/:zandre-XPS-15-9570:type",
					SourceID:   "urn:stackstate-agent/:zandre-XPS-15-9570",
					TargetID:   "urn:agent-integration/:zandre-XPS-15-9570:type",
					Type:       topology.Type{Name: "runs"},
					Data:       map[string]interface{}{}}, {
					ExternalID: "urn:agent-integration/:zandre-XPS-15-9570:type-has-urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					SourceID:   "urn:agent-integration/:zandre-XPS-15-9570:type",
					TargetID:   "urn:agent-integration-instance/:zandre-XPS-15-9570:type:url",
					Type:       topology.Type{Name: "has"},
					Data:       map[string]interface{}{},
				},
			},
		},
	}), expectedTopology)
}
