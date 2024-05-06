// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
	vmsgp "github.com/vmihailenco/msgpack/v4"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

type v5Message [2]interface{}

func (msg v5Message) MarshalMsg([]byte) ([]byte, error) {
	return vmsgp.Marshal(&msg)
}

func TestCreditCards(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	for _, tt := range []struct {
		conf    []byte
		out     string
		version api.Version
	}{
		{
			conf: []byte(`
apm_config:
  env: my-env
  obfuscation:
    credit_cards:
      enabled: false`),
			out:     "4166 6766 6766 6746",
			version: "v0.4",
		},
		{
			conf:    []byte(``),
			out:     "?",
			version: "v0.4",
		},
		{
			conf: []byte(`
apm_config:
  env: my-env
  obfuscation:
    credit_cards:
      enabled: false`),
			out:     "4166 6766 6766 6746",
			version: "v0.5",
		},
		{
			conf:    []byte(``),
			out:     "?",
			version: "v0.5",
		},
		{
			conf: []byte(`
apm_config:
  env: my-env
  obfuscation:
    credit_cards:
      enabled: false`),
			out:     "4166 6766 6766 6746",
			version: "v0.7",
		},
		{
			conf:    []byte(``),
			out:     "?",
			version: "v0.7",
		},
	} {
		t.Run(string(tt.version)+"/"+tt.out, func(t *testing.T) {
			if err := r.RunAgent(tt.conf); err != nil {
				t.Fatal(err)
			}
			defer r.KillAgent()

			payload, traces := generatePayload(tt.version, "4166 6766 6766 6746")
			if err := r.PostMsgpack("/"+string(tt.version)+"/traces", payload); err != nil {
				t.Fatal(err)
			}
			waitForTrace(t, &r, func(v *pb.AgentPayload) {
				payloadsEqual(t, traces, v)
				assert.Equal(t, tt.out, v.TracerPayloads[0].Chunks[0].Spans[0].Meta["credit_card_number"])
			})
		})
	}
}

func generatePayload(version api.Version, cardNumber string) (msgp.Marshaler, pb.Traces) {
	traces := testutil.GeneratePayload(1, &testutil.TraceConfig{
		MaxSpans: 1,
		Keep:     true,
	}, &testutil.SpanConfig{MinTags: 2})
	traces[0][0].Meta["credit_card_number"] = cardNumber
	switch version {
	case "v0.4":
		return traces, traces
	case "v0.5":
		return generateV5Payload(cardNumber)
	case "v0.7":
		return testutil.TracerPayloadWithChunk(testutil.TraceChunkWithSpans(traces[0])), traces
	default:
		panic("invalid version")
	}
}

func generateV5Payload(cardNumber string) (msgp.Marshaler, pb.Traces) {
	var payload v5Message = [2]interface{}{
		0: []string{
			0:  "baggage",
			1:  "item",
			2:  "elasticsearch.version",
			3:  "7.0",
			4:  "my_name",
			5:  "_sampling_priority_v1",
			6:  "my_service",
			7:  "my_resource",
			8:  "_dd.sampling_rate_whatever",
			9:  "value whatever",
			10: "sql",
			11: "credit_card_number",
			12: cardNumber,
		},
		1: [][][12]interface{}{
			{
				{
					6,
					4,
					7,
					uint64(1),
					uint64(2),
					uint64(3),
					int64(1636672007510577544),
					int64(456),
					1,
					map[interface{}]interface{}{
						8:  9,
						0:  1,
						2:  3,
						11: 12,
					},
					map[interface{}]float64{
						5: 1,
					},
					10,
				},
			},
		},
	}
	traces := pb.Traces{pb.Trace{
		&pb.Span{
			Service:  "my_service",
			Name:     "my_name",
			Resource: "my_resource",
			TraceID:  1,
			SpanID:   2,
			ParentID: 3,
			Start:    1636672007510577544,
			Duration: 456,
			Error:    1,
			Meta: map[string]string{
				"baggage":                    "item",
				"elasticsearch.version":      "7.0",
				"_dd.sampling_rate_whatever": "value whatever",
				"credit_card_number":         cardNumber,
			},
			Metrics: map[string]float64{"_sampling_priority_v1": 1},
			Type:    "sql",
		},
	}}
	return payload, traces
}
