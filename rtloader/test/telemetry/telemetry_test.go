package testtelemetry

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"

	"github.com/StackVista/stackstate-agent/rtloader/test/helpers"
)

func TestMain(m *testing.M) {
	err := setUp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up tests: %v", err)
		os.Exit(-1)
	}

	os.Exit(m.Run())
}

func TestSubmitTopologyEvent(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`telemetry.submit_topology_event(
							None,
							"checkid",
							{
								"msg_title": "ev_title",
								"msg_text": "ev_text",
								"timestamp": 21,
								"priority": "ev_priority",
								"host": "ev_host",
								"tags": ["tag1", "tag2"],
								"alert_type": "ev_alert_type",
								"aggregation_key": "ev_aggregation_key",
								"source_type_name": "ev_source_type_name",
								"event_type": "ev_event_type",
								"context": {
								  "source_identifier": "ctx_source_id",
								  "element_identifiers": ["ctx_elem_id1", "ctx_elem_id2"],
								  "source": "ctx_source",
								  "category": "ctx_category",
								  "data": {
									"some": "data"
								  },
								  "source_links": [
                                   {"title": "source0_title", "url": "source0_url"},
                                   {"title": "source1_title", "url": "source1_url"},
                                   {"title": "source2_title", "url": "source2_url"},
								  ]
								}
							})
				`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}

	if _topoEvt.Title != "ev_title" {
		t.Fatalf("Unexpected topology event data 'msg_title' value: %s", _topoEvt.Title)
	}
	if _topoEvt.Text != "ev_text" {
		t.Fatalf("Unexpected topology event data 'msg_text' value: %s", _topoEvt.Text)
	}
	if _topoEvt.Ts != 21 {
		t.Fatalf("Unexpected topology event data 'timestamp' value: %d", _topoEvt.Ts)
	}
	if _topoEvt.Priority != "ev_priority" {
		t.Fatalf("Unexpected topology event data 'priority' value: %s", _topoEvt.Priority)
	}
	if _topoEvt.Host != "ev_host" {
		t.Fatalf("Unexpected topology event data 'host' value: %s", _topoEvt.Host)
	}
	if len(_topoEvt.Tags) != 2 {
		t.Fatalf("Unexpected topology event data 'tags' size: %v", len(_topoEvt.Tags))
	}
	if _topoEvt.Tags[0] != "tag1" && _topoEvt.Tags[1] != "tag2" {
		t.Fatalf("Unexpected topology event data 'tags' value: %s", _topoEvt.Tags)
	}
	if _topoEvt.AlertType != "ev_alert_type" {
		t.Fatalf("Unexpected topology event data 'alert_type' value: %s", _topoEvt.AlertType)
	}
	if _topoEvt.AggregationKey != "ev_aggregation_key" {
		t.Fatalf("Unexpected topology event data 'aggregation_key' value: %s", _topoEvt.AggregationKey)
	}
	if _topoEvt.SourceTypeName != "ev_source_type_name" {
		t.Fatalf("Unexpected topology event data 'source_type_name' value: %s", _topoEvt.SourceTypeName)
	}
	if _topoEvt.EventType != "ev_event_type" {
		t.Fatalf("Unexpected topology event data 'event_type' value: %s", _topoEvt.EventType)
	}
	if _topoEvt.EventContext.SourceIdentifier != "ctx_source_id" {
		t.Fatalf("Unexpected topology event data 'context.source_identifier' value: %s", _topoEvt.EventContext.SourceIdentifier)
	}
	if len(_topoEvt.EventContext.ElementIdentifiers) != 2 {
		t.Fatalf("Unexpected topology event data 'context.element_identifiers' size: %v", len(_topoEvt.EventContext.ElementIdentifiers))
	}
	if _topoEvt.EventContext.ElementIdentifiers[0] != "ctx_elem_id1" && _topoEvt.EventContext.ElementIdentifiers[1] != "ctx_elem_id2" {
		t.Fatalf("Unexpected topology event data 'context.element_identifiers' value: %s", _topoEvt.EventContext.ElementIdentifiers)
	}
	if _topoEvt.EventContext.Source != "ctx_source" {
		t.Fatalf("Unexpected topology event data 'context.source' value: %s", _topoEvt.EventContext.Source)
	}
	if _topoEvt.EventContext.Category != "ctx_category" {
		t.Fatalf("Unexpected topology event data 'context.category' value: %s", _topoEvt.EventContext.Category)
	}
	if _topoEvt.EventContext.Data["some"] != "data" {
		t.Fatalf("Unexpected topology event data 'context.data' value: %s", _topoEvt.EventContext.Data["some"])
	}
	if len(_topoEvt.EventContext.SourceLinks) != 3 {
		t.Fatalf("Unexpected topology event data 'context.source_links' size: %v", len(_topoEvt.EventContext.SourceLinks))
	}
	for i := 0; i < len(_topoEvt.EventContext.SourceLinks); i++ {
		var sourceLink = _topoEvt.EventContext.SourceLinks[i]
		if sourceLink.Title != fmt.Sprintf("source%v_title", i) {
			t.Fatalf("Unexpected topology event data 'context.source_links[%v].title' value: %s", i, sourceLink.Title)
		}
		if sourceLink.URL != fmt.Sprintf("source%v_url", i) {
			t.Fatalf("Unexpected topology event data 'context.source_links[%v].url' value: %s", i, sourceLink.URL)
		}
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubmitTopologyChangeRequestEvents(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`telemetry.submit_topology_event(
							None,
							"checkid",
							{
								'event_type': 'Change Request Normal',
								'tags': ['number:CHG0000001', 'priority:3 - Moderate', 'risk:High', 'state:New', 'category:Software', 'conflict_status:None', 'assigned_to:ITIL User'],
								'timestamp': 1600951343,
								'msg_title': 'CHG0000001: Rollback Oracle Version',
								'msg_text': 'Performance of the Siebel SFA software has been severely\n            degraded since the upgrade performed this weekend.\n\n            We moved to an unsupported Oracle DB version. Need to rollback the\n            Oracle Instance to a supported version.\n        ',
								'context': {
									'category': 'change_request',
									'source': 'servicenow',
									'data': {'impact': '3 - Low', 'requested_by': 'David Loo'},
									'element_identifiers': ['a9c0c8d2c6112276018f7705562f9cb0', 'urn:host:/Sales \xc2\xa9 Force Automation', 'urn:host:/sales \xc2\xa9 force automation'],
									'source_links': []
								},
								'source_type_name': 'Change Request Normal'
							}
				)
				`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}

	if _topoEvt.Text != "Performance of the Siebel SFA software has been severely\n            degraded since the upgrade performed this weekend.\n\n            We moved to an unsupported Oracle DB version. Need to rollback the\n            Oracle Instance to a supported version.\n        " {
		t.Fatalf("Unexpected topology event data 'msg_text' value: '%s'", _topoEvt.Text)
	}
	if _topoEvt.Ts != 1600951343 {
		t.Fatalf("Unexpected topology event data 'timestamp' value: %d", _topoEvt.Ts)
	}
	if len(_topoEvt.Tags) != 7 {
		t.Fatalf("Unexpected topology event data 'tags' size: %v", len(_topoEvt.Tags))
	}
	if _topoEvt.Tags[0] != "number:CHG0000001" && _topoEvt.Tags[1] != "priority:3 - Moderate" && _topoEvt.Tags[2] != "risk:High" &&
		_topoEvt.Tags[3] != "state:New" && _topoEvt.Tags[4] != "category:Software" && _topoEvt.Tags[5] != "conflict_status:None" &&
		_topoEvt.Tags[6] != "assigned_to:ITIL User" {
		t.Fatalf("Unexpected topology event data 'tags' value: %s", _topoEvt.Tags)
	}
	if _topoEvt.EventType != "Change Request Normal" {
		t.Fatalf("Unexpected topology event data 'event_type' value: %s", _topoEvt.EventType)
	}
	if len(_topoEvt.EventContext.ElementIdentifiers) != 3 {
		t.Fatalf("Unexpected topology event data 'context.element_identifiers' size: %v", len(_topoEvt.EventContext.ElementIdentifiers))
	}
	if _topoEvt.EventContext.ElementIdentifiers[0] != "a9c0c8d2c6112276018f7705562f9cb0" &&
		_topoEvt.EventContext.ElementIdentifiers[1] != "urn:host:/Sales \xc2\xa9 Force Automation" &&
		_topoEvt.EventContext.ElementIdentifiers[2] != "urn:host:/sales \xc2\xa9 force automation" {
		t.Fatalf("Unexpected topology event data 'context.element_identifiers' value: %s", _topoEvt.EventContext.ElementIdentifiers)
	}
	if _topoEvt.EventContext.Source != "servicenow" {
		t.Fatalf("Unexpected topology event data 'context.source' value: %s", _topoEvt.EventContext.Source)
	}
	if _topoEvt.EventContext.Category != "change_request" {
		t.Fatalf("Unexpected topology event data 'context.category' value: %s", _topoEvt.EventContext.Category)
	}
	if _topoEvt.EventContext.Data["impact"] != "3 - Low" {
		t.Fatalf("Unexpected topology event data 'context.data.impact' value: %s", _topoEvt.EventContext.Data["impact"])
	}
	if _topoEvt.EventContext.Data["requested_by"] != "David Loo" {
		t.Fatalf("Unexpected topology event data 'context.data.requested_by' value: %s", _topoEvt.EventContext.Data["requested_by"])
	}
	if len(_topoEvt.EventContext.SourceLinks) != 0 {
		t.Fatalf("Unexpected topology event data 'context.source_links' size: %v", len(_topoEvt.EventContext.SourceLinks))
	}

	emptySourceLinks := make([]metrics.SourceLink, 0)
	if !assert.ObjectsAreEqualValues(_topoEvt.EventContext.SourceLinks, emptySourceLinks) {
		t.Fatalf("Unexpected topology event data 'context.source_links' value: %v", _topoEvt.EventContext.SourceLinks)
	}

	if _topoEvt.Title != "CHG0000001: Rollback Oracle Version" {
		t.Fatalf("Unexpected topology event data 'msg_title' value: '%s'", _topoEvt.Title)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubmitTopologyEventEmptyDict(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`telemetry.submit_topology_event(None, "checkid", {})`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("Unexpected printed value: '%s'", out)
	}

	if checkID != "checkid" {
		t.Fatalf("Unexpected check id value: %s", checkID)
	}

	if len(_data) != 0 {
		t.Fatalf("Unexpected topology event data value: %s", _data)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubmitTopologyEventNoDict(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`telemetry.submit_topology_event(None, "checkid", "I should be a dict")`)

	if err != nil {
		t.Fatal(err)
	}
	if out != "TypeError: topology event must be a dict" {
		t.Errorf("wrong printed value: '%s'", out)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}

func TestSubmitEventCannotBeSerialized(t *testing.T) {
	// Reset memory counters
	helpers.ResetMemoryStats()

	out, err := run(`telemetry.submit_topology_event(None, "checkid", {object(): object()} )`)

	if err != nil {
		t.Fatal(err)
	}
	// keys must be a string
	if !strings.Contains(out, "keys must be") {
		t.Errorf("Unexpected printed value: '%s'", out)
	}
	if len(_data) != 0 {
		t.Fatalf("Unexpected topology event data value: %s", _data)
	}

	// Check for leaks
	helpers.AssertMemoryUsage(t)
}
