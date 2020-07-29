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
								"stream_id": -1, "name": "Integration Health", "is_service_check_health_check":int64(1),
							},
						},
						"events": []interface{}{map[string]interface{}{
							"stream_id": -1,
							"identifier": "c50fcf43-38ca-46b5-b217-52896e5709be",
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
								"stream_id":                     -1,
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
								"stream_id":  -1,
								"identifier": "c50fcf43-38ca-46b5-b217-52896e5709be",
							},
						},
					},
				}, {
					ExternalID: "myid",
					Type:       topology.Type{Name: "mytype"},
					Data: topology.Data{
						"nestedobject": map[string]interface{}{"nestedkey": "nestedValue"},
						"key":          "value",
						"intlist":      []interface{}{1}, "emptykey": map[string]interface{}{},
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
	}), mockBatcher.CollectedTopology.Flush())
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
							map[string]interface{}{"stream_id": -1, "name": "Integration Health", "is_service_check_health_check":int64(1)},
						},
						"events": []interface{}{
							map[string]interface{}{
								"conditions": []interface{}{
									map[string]interface{}{"value": "zandre-XPS-15-9570", "key": "hostname"},
									map[string]interface{}{"value": "type", "key": "integration-type"},
								},
								"name":       "Service Checks",
								"stream_id":  -1,
								"identifier": "d8e115f4-3832-4c17-953c-727954311734",
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
								"stream_id":                     -1,
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
								"stream_id":  -1,
								"identifier": "0cec1c40-dc23-4b4a-b2e1-2c07a9045ffa"},
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
						"intlist":      []interface{}{1},
						"emptykey":     map[string]interface{}{},
					},
				},
			},
		},
	}), mockBatcher.CollectedTopology.Flush())
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
							map[string]interface{}{"name": "Integration Health", "is_service_check_health_check":int64(1), "stream_id": -1},
						},
						"events": []interface{}{
							map[string]interface{}{
								"stream_id":  int64(-1),
								"identifier": "d15fc8bf-9c86-44e0-b6c5-074027fa2d7e",
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
								"identifier": "794c8bac-1c7c-4674-ae3a-b8bee0bc0d48",
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
	}), mockBatcher.CollectedTopology.Flush())
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
							map[string]interface{}{"name": "Integration Health", "is_service_check_health_check":int64(1), "stream_id": -1},
						},
						"events": []interface{}{
							map[string]interface{}{
								"identifier": "ecd18964-74d5-40d2-be80-744aaa443f93",
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
								"identifier": "eaab58cb-f95c-412c-860b-621b944f9b0e",
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
	}), mockBatcher.CollectedTopology.Flush())
}
