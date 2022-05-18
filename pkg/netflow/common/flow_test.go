package common

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFlow_AggregationHash(t *testing.T) {
	origFlow := Flow{
		SrcAddr:        "1.2.3.4",
		DstAddr:        "2.3.4.5",
		IPProtocol:     6,
		SrcPort:        2000,
		DstPort:        80,
		InputInterface: 1,
		Tos:            0,
	}
	origHash := origFlow.AggregationHash()
	assert.Equal(t, "f1c7f3f1048a8e6", origHash)

	flow := origFlow
	flow.SrcAddr = "1.2.3.5"
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.DstAddr = "2.3.4.6"
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.IPProtocol = 7
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.SrcPort = 3000
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.DstPort = 443
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.InputInterface = 2
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.Tos = 1
	assert.NotEqual(t, origHash, flow.AggregationHash())

	// OutputInterface is not a key field, changing it should not change the hash
	flow = origFlow
	flow.OutputInterface = 1
	assert.Equal(t, origHash, flow.AggregationHash())

	// EtherType is not a key field, changing it should not change the hash
	flow = origFlow
	flow.EtherType = 1
	assert.Equal(t, origHash, flow.AggregationHash())
}

func TestFlow_AsJSONString(t *testing.T) {
	origFlow := Flow{
		Namespace:      "my-ns",
		FlowType:       TypeNetFlow9,
		SrcAddr:        "1.2.3.4",
		DstAddr:        "2.3.4.5",
		ExporterAddr:   "127.0.0.1",
		IPProtocol:     6,
		SrcPort:        2000,
		DstPort:        80,
		InputInterface: 1,
		Tos:            0,
	}
	// language=json
	expectedJSON := `{
    "namespace":"my-ns",
    "type":"netflow9",
    "sampling_rate":0,
    "direction":0,
    "exporter_addr":"127.0.0.1",
    "start_timestamp":0,
    "end_timestamp":0,
    "bytes":0,
    "packets":0,
    "src_addr":"1.2.3.4",
    "dst_addr":"2.3.4.5",
    "ether_type":0,
    "ip_protocol":6,
    "src_port":2000,
    "dst_port":80,
    "input_interface":1,
    "output_interface":0,
    "src_mac":0,
    "dst_mac":0,
    "src_mask":0,
    "dst_mask":0,
    "tos":0,
    "next_hop":""
}`
	var expectedPretty bytes.Buffer
	err := json.Indent(&expectedPretty, []byte(expectedJSON), "", "\t")
	assert.NoError(t, err)

	var actualPretty bytes.Buffer
	err = json.Indent(&actualPretty, []byte(origFlow.AsJSONString()), "", "\t")
	assert.NoError(t, err)

	assert.Equal(t, expectedPretty.String(), actualPretty.String())
}

func TestFlow_TelemetryTags(t *testing.T) {
	flow := Flow{
		FlowType:       TypeNetFlow9,
		SrcAddr:        "1.2.3.4",
		DstAddr:        "2.3.4.5",
		ExporterAddr:   "127.0.0.1",
		IPProtocol:     6,
		SrcPort:        2000,
		DstPort:        80,
		InputInterface: 1,
		Tos:            0,
		Namespace:      "my-ns",
	}
	expectedTags := []string{"exporter:127.0.0.1", "flow_type:netflow9", "namespace:my-ns"}
	assert.ElementsMatch(t, expectedTags, flow.TelemetryTags())
}
