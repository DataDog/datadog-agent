// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// NDMFlow represents an ndmflow payload
type NDMFlow struct {
	collectedTime time.Time
	//JMW from ~/dd/datadog-agent/comp/netflow/payload/payload.go::FlowPayload
	FlushTimestamp int64  `json:"flush_timestamp"`
	FlowType       string `json:"type"`
	SamplingRate   uint64 `json:"sampling_rate"`
	Direction      string `json:"direction"`
	Start          uint64 `json:"start"` // in seconds
	End            uint64 `json:"end"`   // in seconds
	Bytes          uint64 `json:"bytes"`
	Packets        uint64 `json:"packets"`
	EtherType      string `json:"ether_type,omitempty"`
	IPProtocol     string `json:"ip_protocol"`
	/*JMW
	Device           Device           `json:"device"`
	Exporter         Exporter         `json:"exporter"`
	Source           Endpoint         `json:"source"`
	Destination      Endpoint         `json:"destination"`
	Ingress          ObservationPoint `json:"ingress"`
	Egress           ObservationPoint `json:"egress"`
	*/
	Host     string   `json:"host"`
	TCPFlags []string `json:"tcp_flags,omitempty"`
	/*JMW
	NextHop          NextHop          `json:"next_hop,omitempty"`
	AdditionalFields AdditionalFields `json:"additional_fields,omitempty"`
	*/
}

/*JMW
func (t *tags) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*t = tags(strings.Split(s, ","))
	return nil
}

func (t tags) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.Join(t, ","))
}
*/

func (p *NDMFlow) name() string {
	return "ndmflow"
}

// GetTags return the tags from a payload
func (p *NDMFlow) GetTags() []string {
	return []string{}
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (p *NDMFlow) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseNDMFlowPayload return the parsed ndmflows from payload
func ParseNDMFlowPayload(payload api.Payload) (ndmflows []*NDMFlow, err error) {
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		// logs can submit with empty data or empty JSON object
		return []*NDMFlow{}, nil
	}
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	ndmflows = []*NDMFlow{}
	err = json.Unmarshal(enflated, &ndmflows)
	if err != nil {
		return nil, err
	}
	for _, n := range ndmflows {
		n.collectedTime = payload.Timestamp
	}
	return ndmflows, err

}

// NDMFlowAggregator is an Aggregator for ndmflow payloads
type NDMFlowAggregator struct {
	Aggregator[*NDMFlow]
}

// NewNDMFlowAggregator return a new NDMFlowAggregator
func NewNDMFlowAggregator() NDMFlowAggregator {
	return NDMFlowAggregator{
		Aggregator: newAggregator(ParseNDMFlowPayload),
	}
}
