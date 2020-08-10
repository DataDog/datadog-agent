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
	instance := topology.Instance{Type: "type", URL: "url"}
	components, relations := getAgentIntegrationTopology(t, "type:url", instance, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      instance,
			Components: []topology.Component{
				{
					ExternalID: "myid",
					Type:       topology.Type{Name: "mytype"},
					Data: topology.Data{
						"nestedobject": map[string]interface{}{"nestedkey": "nestedValue"},
						"key":          "value",
						"intlist":      []interface{}{int64(1)}, "emptykey": map[string]interface{}{},
						"tags": []interface{}{
							fmt.Sprintf("integration-type:%s", instance.Type),
							fmt.Sprintf("integration-url:%s", instance.URL),
						},
					},
				},
			},
			Relations: []topology.Relation{},
		},
		"type:url": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "agent", URL: "integrations"},
			Components:    components,
			Relations:     relations,
		},
	}), expectedTopology)
}

func TestRelationTopology(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestRelationCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	expectedTopology := mockBatcher.CollectedTopology.Flush()
	instance := topology.Instance{Type: "type", URL: "url"}
	components, relations := getAgentIntegrationTopology(t, "type:url", instance, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      instance,
			Components:    []topology.Component{},
			Relations: []topology.Relation{
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
			},
		},
		"type:url": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "agent", URL: "integrations"},
			Components:    components,
			Relations:     relations,
		},
	}), expectedTopology)
}

func TestStartSnapshotCheck(t *testing.T) {
	chk, _ := getCheckInstance("testtopology", "TestStartSnapshotCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	expectedTopology := mockBatcher.CollectedTopology.Flush()
	instance := topology.Instance{Type: "type", URL: "url"}
	components, relations := getAgentIntegrationTopology(t, "type:url", instance, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      instance,
			Components:    []topology.Component{},
			Relations:     []topology.Relation{},
		},
		"type:url": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "agent", URL: "integrations"},
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
	instance := topology.Instance{Type: "type", URL: "url"}
	components, relations := getAgentIntegrationTopology(t, "type:url", instance, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testtopology:c3d960f8ff8a5c55": {
			StartSnapshot: true,
			StopSnapshot:  true,
			Instance:      instance,
			Components:    []topology.Component{},
			Relations:     []topology.Relation{},
		},
		"type:url": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "agent", URL: "integrations"},
			Components:    components,
			Relations:     relations,
		},
	}), expectedTopology)
}

func TestAgentIntegration(t *testing.T) {
	chk, _ := getCheckInstance("testcheck_agent_integration", "AgentIntegrationSampleCheck")

	mockBatcher := batcher.NewMockBatcher()

	err := chk.Run()
	assert.Nil(t, err)
	expectedTopology := mockBatcher.CollectedTopology.Flush()
	t.Logf("ExpectedTopology: %v\n", expectedTopology)
	instance := topology.Instance{Type: "type", URL: "url"}
	var hostCPUUsageIdentifier, responses2xxIdentifier, responses5xxIdentifier string
	for _, e := range expectedTopology["testcheck_agent_integration:c3d960f8ff8a5c55"].Components[3].Data["metrics"].([]interface{}) {
		hostCPUUsageIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	for i, e := range expectedTopology["testcheck_agent_integration:c3d960f8ff8a5c55"].Components[4].Data["metrics"].([]interface{}) {
		if i == 0 {
			responses2xxIdentifier = e.(map[string]interface{})["identifier"].(string)
		}

		if i == 1 {
			responses5xxIdentifier = e.(map[string]interface{})["identifier"].(string)
		}
	}

	components, relations := getAgentIntegrationTopology(t, "testcheck_agent_integration:c3d960f8ff8a5c55", instance, expectedTopology)

	assert.Equal(t, batcher.Topologies(map[check.ID]topology.Topology{
		"testcheck_agent_integration:c3d960f8ff8a5c55": {
			StartSnapshot: false,
			StopSnapshot:  false,
			Instance:      topology.Instance{Type: "agent", URL: "integrations"},
			Components: append(components, []topology.Component{
				{
					ExternalID: "urn:example:/host:this_host",
					Type:       topology.Type{Name: "Host"},
					Data: topology.Data{
						"name":   "this-host",
						"labels": []interface{}{"host:this_host", "region:eu-west-1"},
						"tags": []interface{}{
							fmt.Sprintf("integration-type:%s", instance.Type),
							fmt.Sprintf("integration-url:%s", instance.URL),
						},
						"layer":       "Hosts",
						"environment": "Production",
						"checks": []interface{}{
							map[string]interface{}{
								"stream_id":                       int64(-1),
								"max_window":                      int64(300000),
								"name":                            "Max CPU Usage (Average)",
								"deviating_value":                 int64(75),
								"is_metric_maximum_average_check": int64(1),
								"critical_value":                  int64(90),
							},
							map[string]interface{}{
								"name":                         "Max CPU Usage (Last)",
								"deviating_value":              int64(75),
								"is_metric_maximum_last_check": int64(1),
								"critical_value":               int64(90),
								"stream_id":                    int64(-1),
								"max_window":                   int64(300000),
							},
							map[string]interface{}{
								"deviating_value":                 int64(10),
								"is_metric_minimum_average_check": int64(1),
								"critical_value":                  int64(5),
								"stream_id":                       int64(-1),
								"max_window":                      int64(300000),
								"name":                            "Min CPU Usage (Average)",
							},
							map[string]interface{}{
								"is_metric_minimum_last_check": int64(1),
								"critical_value":               int64(5),
								"stream_id":                    int64(-1),
								"max_window":                   int64(300000),
								"name":                         "Min CPU Usage (Last)",
								"deviating_value":              int64(10),
							},
						},
						"metrics": []interface{}{
							map[string]interface{}{
								"unit_of_measure": "Percentage",
								"name":            "Host CPU Usage",
								"identifier":      hostCPUUsageIdentifier,
								"conditions": []interface{}{
									map[string]interface{}{"value": "this-host", "key": "tags.hostname"},
								},
								"aggregation":  "MEAN",
								"metric_field": "host.cpu.usage",
								"priority":     "HIGH",
								"stream_id":    int64(-1),
							},
						},
						"domain":      "Webshop",
						"identifiers": []interface{}{"another_identifier_for_this_host"}},
				},
				{
					ExternalID: "urn:example:/application:some_application",
					Type:       topology.Type{Name: "Application"},
					Data: topology.Data{
						"labels": []interface{}{"application:some_application", "region:eu-west-1", "hosted_on:this-host"},
						"tags": []interface{}{
							fmt.Sprintf("integration-type:%s", instance.Type),
							fmt.Sprintf("integration-url:%s", instance.URL),
						},
						"environment": "Production",
						"version":     "0.2.0",
						"name":        "some-application",
						"layer":       "Applications",
						"checks": []interface{}{
							map[string]interface{}{
								"max_window":                    int64(300000),
								"name":                          "OK vs Error Responses (Maximum)",
								"deviating_value":               int64(50),
								"is_metric_maximum_ratio_check": int64(1),
								"numerator_stream_id":           int64(-2),
								"denominator_stream_id":         int64(-1),
								"critical_value":                int64(75),
							},
							map[string]interface{}{
								"name":                               "Error Response 99th Percentile",
								"deviating_value":                    int64(50),
								"is_metric_maximum_percentile_check": int64(1),
								"percentile":                         int64(99),
								"critical_value":                     int64(70),
								"stream_id":                          int64(-2),
								"max_window":                         int64(300000),
							},
							map[string]interface{}{
								"numerator_stream_id":          int64(-2),
								"denominator_stream_id":        int64(-1),
								"critical_value":               int64(75),
								"max_window":                   int64(300000),
								"name":                         "OK vs Error Responses (Failed)",
								"deviating_value":              int64(50),
								"is_metric_failed_ratio_check": int64(1),
							},
							map[string]interface{}{
								"stream_id":                          int64(-1),
								"max_window":                         int64(300000),
								"name":                               "Success Response 99th Percentile",
								"deviating_value":                    int64(10),
								"is_metric_minimum_percentile_check": int64(1),
								"percentile":                         int64(99),
								"critical_value":                     int64(5),
							},
						},
						"metrics": []interface{}{
							map[string]interface{}{
								"priority":        "HIGH",
								"stream_id":       int64(-1),
								"unit_of_measure": "Count",
								"name":            "2xx Responses",
								"identifier":      responses2xxIdentifier,
								"conditions": []interface{}{
									map[string]interface{}{"value": "some_application", "key": "tags.application"},
									map[string]interface{}{"value": "eu-west-1", "key": "tags.region"},
								},
								"aggregation":  "MEAN",
								"metric_field": "2xx.responses",
							},
							map[string]interface{}{
								"identifier": responses5xxIdentifier,
								"conditions": []interface{}{
									map[string]interface{}{"value": "some_application", "key": "tags.application"},
									map[string]interface{}{"value": "eu-west-1", "key": "tags.region"},
								},
								"aggregation":     "MEAN",
								"metric_field":    "5xx.responses",
								"priority":        "HIGH",
								"stream_id":       int64(-2),
								"unit_of_measure": "Count",
								"name":            "5xx Responses",
							},
						},
						"domain":      "Webshop",
						"identifiers": []interface{}{"another_identifier_for_some_application"},
					},
				},
			}...),
			Relations: append(relations, topology.Relation{
				ExternalID: "urn:example:/application:some_application-IS_HOSTED_ON-urn:example:/host:this_host",
				SourceID:   "urn:example:/application:some_application",
				TargetID:   "urn:example:/host:this_host",
				Type:       topology.Type{Name: "IS_HOSTED_ON"},
				Data:       map[string]interface{}{},
			}),
		},
	}), expectedTopology)
}

func getAgentIntegrationTopology(t *testing.T, key check.ID, instance topology.Instance, expectedTopology batcher.Topologies) ([]topology.Component, []topology.Relation) {
	var integrationIdentifier, integrationInstanceIdentifier string
	for _, e := range expectedTopology[key].Components[1].Data["service_checks"].([]interface{}) {
		integrationIdentifier = e.(map[string]interface{})["identifier"].(string)
	}
	for _, e := range expectedTopology[key].Components[2].Data["service_checks"].([]interface{}) {
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
					"tags": []interface{}{
						fmt.Sprintf("hostname:%s", host),
						"stackstate-agent",
					},
				},
			},
			{
				ExternalID: fmt.Sprintf("urn:agent-integration:/%s:%s", host, instance.Type),
				Type:       topology.Type{Name: "agent-integration"},
				Data: topology.Data{
					"tags": []interface{}{
						fmt.Sprintf("hostname:%s", host),
						fmt.Sprintf("integration-type:%s", instance.Type),
					},
					"hostname":    host,
					"integration": instance.Type,
					"checks": []interface{}{
						map[string]interface{}{
							"stream_id": int64(-1), "name": "Integration Health", "is_service_check_health_check": int64(1),
						},
					},
					"service_checks": []interface{}{map[string]interface{}{
						"stream_id":  int64(-1),
						"identifier": integrationIdentifier,
						"conditions": []interface{}{
							map[string]interface{}{"value": host, "key": "host"},
							map[string]interface{}{"value": instance.Type, "key": "tags.integration-type"},
						},
						"name": "Service Checks",
					}},
					"name": fmt.Sprintf("%s:%s", host, instance.Type),
				},
			},
			{
				ExternalID: fmt.Sprintf("urn:agent-integration-instance:/%s:%s:%s", host, instance.Type, instance.URL),
				Type:       topology.Type{Name: "agent-integration-instance"},
				Data: topology.Data{
					"name": fmt.Sprintf("%s:%s", instance.Type, instance.URL),
					"tags": []interface{}{
						fmt.Sprintf("hostname:%s", host),
						fmt.Sprintf("integration-type:%s", instance.Type),
						fmt.Sprintf("integration-url:%s", instance.URL),
					},
					"hostname":    host,
					"integration": instance.Type,
					"checks": []interface{}{
						map[string]interface{}{
							"stream_id":                     int64(-1),
							"name":                          "Integration Instance Health",
							"is_service_check_health_check": int64(1),
						},
					},
					"service_checks": []interface{}{
						map[string]interface{}{
							"conditions": []interface{}{
								map[string]interface{}{"value": host, "key": "host"},
								map[string]interface{}{"value": instance.Type, "key": "tags.integration-type"},
								map[string]interface{}{"value": instance.URL, "key": "tags.integration-url"},
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
				ExternalID: fmt.Sprintf("urn:stackstate-agent:/%s-runs-urn:agent-integration:/%s:%s", host, host, instance.Type),
				SourceID:   fmt.Sprintf("urn:stackstate-agent:/%s", host),
				TargetID:   fmt.Sprintf("urn:agent-integration:/%s:%s", host, instance.Type),
				Type:       topology.Type{Name: "runs"}, Data: map[string]interface{}{},
			}, {
				ExternalID: fmt.Sprintf("urn:agent-integration:/%s:%s-has-urn:agent-integration-instance:/%s:%s:%s",
					host, instance.Type, host, instance.Type, instance.URL),
				SourceID: fmt.Sprintf("urn:agent-integration:/%s:%s", host, instance.Type),
				TargetID: fmt.Sprintf("urn:agent-integration-instance:/%s:%s:%s", host, instance.Type, instance.URL),
				Type:     topology.Type{Name: "has"},
				Data:     map[string]interface{}{},
			},
		}
}
