package payload

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFlowPayload_MarshalJSON(t *testing.T) {
	flow := FlowPayload{
		FlushTimestamp: 123,
		AdditionalFields: map[string]any{
			"my-int": 1234,
			"my-str": "abc",
		},
	}
	marshal, err := json.MarshalIndent(flow, "", "   ")
	require.NoError(t, err)
	fmt.Println(string(marshal))
	assert.Equal(t, `{
   "bytes": 0,
   "destination": {
      "ip": "",
      "mac": "",
      "mask": "",
      "port": ""
   },
   "device": {
      "namespace": ""
   },
   "direction": "",
   "egress": {
      "interface": {
         "index": 0
      }
   },
   "end": 0,
   "exporter": {
      "ip": ""
   },
   "flush_timestamp": 123,
   "host": "",
   "ingress": {
      "interface": {
         "index": 0
      }
   },
   "ip_protocol": "",
   "my-int": 1234,
   "my-str": "abc",
   "next_hop": {
      "ip": ""
   },
   "packets": 0,
   "sampling_rate": 0,
   "source": {
      "ip": "",
      "mac": "",
      "mask": "",
      "port": ""
   },
   "start": 0,
   "type": ""
}`, string(marshal))
}
