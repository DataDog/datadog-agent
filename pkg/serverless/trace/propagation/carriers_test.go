// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package propagation

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"

	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

func getMapFromCarrier(tm ddtrace.TextMapReader) map[string]string {
	if tm == nil {
		return nil
	}
	m := map[string]string{}
	tm.ForeachKey(func(key, val string) error {
		m[key] = val
		return nil
	})
	return m
}

func TestSQSMessageAttrCarrier(t *testing.T) {
	testcases := []struct {
		name     string
		attr     events.SQSMessageAttribute
		expMap   map[string]string
		expNoErr bool
	}{
		{
			name: "string-datadog-map",
			attr: events.SQSMessageAttribute{
				DataType:    "String",
				StringValue: aws.String(headersAll),
			},
			expMap:   headersMapAll,
			expNoErr: true,
		},
		{
			name: "string-empty-map",
			attr: events.SQSMessageAttribute{
				DataType:    "String",
				StringValue: aws.String("{}"),
			},
			expMap:   map[string]string{},
			expNoErr: true,
		},
		{
			name: "string-empty-string",
			attr: events.SQSMessageAttribute{
				DataType:    "String",
				StringValue: aws.String(""),
			},
			expMap:   nil,
			expNoErr: false,
		},
		{
			name: "string-nil-string",
			attr: events.SQSMessageAttribute{
				DataType:    "String",
				StringValue: nil,
			},
			expMap:   nil,
			expNoErr: false,
		},
		{
			name: "binary-datadog-map",
			attr: events.SQSMessageAttribute{
				DataType:    "Binary",
				BinaryValue: []byte(headersAll),
			},
			expMap:   headersMapAll,
			expNoErr: true,
		},
		{
			name: "binary-empty-map",
			attr: events.SQSMessageAttribute{
				DataType:    "Binary",
				BinaryValue: []byte("{}"),
			},
			expMap:   map[string]string{},
			expNoErr: true,
		},
		{
			name: "binary-empty-string",
			attr: events.SQSMessageAttribute{
				DataType:    "Binary",
				BinaryValue: []byte(""),
			},
			expMap:   nil,
			expNoErr: false,
		},
		{
			name: "binary-nil-string",
			attr: events.SQSMessageAttribute{
				DataType:    "Binary",
				BinaryValue: nil,
			},
			expMap:   nil,
			expNoErr: false,
		},
		{
			name: "wrong-data-type",
			attr: events.SQSMessageAttribute{
				DataType: "Purple",
			},
			expMap:   nil,
			expNoErr: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tm, err := sqsMessageAttrCarrier(tc.attr)
			t.Logf("sqsMessageAttrCarrier returned TextMapReader=%#v error=%#v", tm, err)
			assert.Equal(t, tc.expNoErr, err == nil)
			assert.Equal(t, tc.expMap, getMapFromCarrier(tm))
		})
	}
}

func TestSnsSqsMessageCarrier(t *testing.T) {
	testcases := []struct {
		name   string
		event  events.SQSMessage
		expMap map[string]string
		expErr string
	}{
		{
			name: "empty-string-body",
			event: events.SQSMessage{
				Body: "",
			},
			expMap: nil,
			expErr: "Error unmarshaling message body:",
		},
		{
			name: "empty-map-body",
			event: events.SQSMessage{
				Body: "{}",
			},
			expMap: nil,
			expErr: "No Datadog trace context found",
		},
		{
			name: "no-msg-attrs",
			event: events.SQSMessage{
				Body: `{
					"MessageAttributes": {}
				}`,
			},
			expMap: nil,
			expErr: "No Datadog trace context found",
		},
		{
			name: "wrong-type-msg-attrs",
			event: events.SQSMessage{
				Body: `{
					"MessageAttributes": "attrs"
				}`,
			},
			expMap: nil,
			expErr: "Error unmarshaling message body:",
		},
		{
			name: "non-binary-type",
			event: events.SQSMessage{
				Body: `{
					"MessageAttributes": {
						"_datadog": {
							"Type": "Purple",
							"Value": "Value"
						}
					}
				}`,
			},
			expMap: nil,
			expErr: "Unsupported Type in _datadog payload",
		},
		{
			name: "cannot-decode",
			event: events.SQSMessage{
				Body: `{
					"MessageAttributes": {
						"_datadog": {
							"Type": "Binary",
							"Value": "Value"
						}
					}
				}`,
			},
			expMap: nil,
			expErr: "Error decoding binary:",
		},
		{
			name: "empty-string-encoded",
			event: events.SQSMessage{
				Body: `{
					"MessageAttributes": {
						"_datadog": {
							"Type": "Binary",
							"Value": "` + base64.StdEncoding.EncodeToString([]byte(``)) + `"
						}
					}
				}`,
			},
			expMap: nil,
			expErr: "Error unmarshaling the decoded binary:",
		},
		{
			name:   "empty-map-encoded",
			event:  eventSqsMessage(headersNone, headersEmpty, headersNone),
			expMap: headersMapEmpty,
			expErr: "",
		},
		{
			name:   "datadog-map",
			event:  eventSqsMessage(headersNone, headersAll, headersNone),
			expMap: headersMapAll,
			expErr: "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tm, err := snsSqsMessageCarrier(tc.event)
			t.Logf("snsSqsMessageCarrier returned TextMapReader=%#v error=%#v", tm, err)
			assert.Equal(t, tc.expErr != "", err != nil)
			if tc.expErr != "" {
				assert.ErrorContains(t, err, tc.expErr)
			}
			assert.Equal(t, tc.expMap, getMapFromCarrier(tm))
		})
	}
}

func TestSnsEntityCarrier(t *testing.T) {
	testcases := []struct {
		name   string
		event  events.SNSEntity
		expMap map[string]string
		expErr string
	}{
		{
			name:   "no-msg-attrs",
			event:  events.SNSEntity{},
			expMap: nil,
			expErr: "No Datadog trace context found",
		},
		{
			name: "wrong-type-msg-attrs",
			event: events.SNSEntity{
				MessageAttributes: map[string]interface{}{
					"_datadog": 12345,
				},
			},
			expMap: nil,
			expErr: "Unsupported type for _datadog payload",
		},
		{
			name: "wrong-type-type",
			event: events.SNSEntity{
				MessageAttributes: map[string]interface{}{
					"_datadog": map[string]interface{}{
						"Type":  12345,
						"Value": "Value",
					},
				},
			},
			expMap: nil,
			expErr: "Unsupported type in _datadog payload",
		},
		{
			name: "wrong-value-type",
			event: events.SNSEntity{
				MessageAttributes: map[string]interface{}{
					"_datadog": map[string]interface{}{
						"Type":  "Binary",
						"Value": 12345,
					},
				},
			},
			expMap: nil,
			expErr: "Unsupported value type in _datadog payload",
		},
		{
			name: "cannot-decode",
			event: events.SNSEntity{
				MessageAttributes: map[string]interface{}{
					"_datadog": map[string]interface{}{
						"Type":  "Binary",
						"Value": "Value",
					},
				},
			},
			expMap: nil,
			expErr: "Error decoding binary: illegal base64 data at input byte 4",
		},
		{
			name: "unknown-type",
			event: events.SNSEntity{
				MessageAttributes: map[string]interface{}{
					"_datadog": map[string]interface{}{
						"Type":  "Purple",
						"Value": "Value",
					},
				},
			},
			expMap: nil,
			expErr: "Unsupported Type in _datadog payload",
		},
		{
			name: "empty-string-encoded",
			event: events.SNSEntity{
				MessageAttributes: map[string]interface{}{
					"_datadog": map[string]interface{}{
						"Type":  "Binary",
						"Value": base64.StdEncoding.EncodeToString([]byte(``)),
					},
				},
			},
			expMap: nil,
			expErr: "Error unmarshaling the decoded binary:",
		},
		{
			name: "binary-type",
			event: events.SNSEntity{
				MessageAttributes: map[string]interface{}{
					"_datadog": map[string]interface{}{
						"Type":  "Binary",
						"Value": base64.StdEncoding.EncodeToString([]byte(headersAll)),
					},
				},
			},
			expMap: headersMapAll,
			expErr: "",
		},
		{
			name: "string-type",
			event: events.SNSEntity{
				MessageAttributes: map[string]interface{}{
					"_datadog": map[string]interface{}{
						"Type":  "String",
						"Value": headersAll,
					},
				},
			},
			expMap: headersMapAll,
			expErr: "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tm, err := snsEntityCarrier(tc.event)
			t.Logf("snsEntityCarrier returned TextMapReader=%#v error=%#v", tm, err)
			assert.Equal(t, tc.expErr != "", err != nil)
			if tc.expErr != "" {
				assert.ErrorContains(t, err, tc.expErr)
			}
			assert.Equal(t, tc.expMap, getMapFromCarrier(tm))
		})
	}
}

func TestExtractTraceContextfromAWSTraceHeader(t *testing.T) {
	ctx := func(trace, parent, priority uint64) *TraceContext {
		return &TraceContext{
			TraceID:          trace,
			ParentID:         parent,
			SamplingPriority: sampler.SamplingPriority(priority),
		}
	}

	testcases := []struct {
		name     string
		value    string
		expTc    *TraceContext
		expNoErr bool
	}{
		{
			name:     "empty string",
			value:    "",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "root but no parent",
			value:    "Root=1-00000000-000000000000000000000001",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "parent but no root",
			value:    "Parent=0000000000000001",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "just root and parent",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000001",
			expTc:    ctx(1, 1, 0),
			expNoErr: true,
		},
		{
			name:     "trailing semi-colon",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;",
			expTc:    ctx(1, 1, 0),
			expNoErr: true,
		},
		{
			name:     "trailing semi-colons",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;;;",
			expTc:    ctx(1, 1, 0),
			expNoErr: true,
		},
		{
			name:     "parent first",
			value:    "Parent=0000000000000009;Root=1-00000000-000000000000000000000001",
			expTc:    ctx(1, 9, 0),
			expNoErr: true,
		},
		{
			name:     "two roots",
			value:    "Root=1-00000000-000000000000000000000005;Parent=0000000000000009;Root=1-00000000-000000000000000000000001",
			expTc:    ctx(5, 9, 0),
			expNoErr: true,
		},
		{
			name:     "two parents",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000009;Parent=0000000000000000",
			expTc:    ctx(1, 9, 0),
			expNoErr: true,
		},
		{
			name:     "sampled 0",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000002;Sampled=0",
			expTc:    ctx(1, 2, 0),
			expNoErr: true,
		},
		{
			name:     "sampled 1",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000002;Sampled=1",
			expTc:    ctx(1, 2, 1),
			expNoErr: true,
		},
		{
			name:     "sampled too big",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000002;Sampled=5",
			expTc:    ctx(1, 2, 0),
			expNoErr: true,
		},
		{
			name:     "sampled first",
			value:    "Sampled=1;Root=1-00000000-000000000000000000000001;Parent=0000000000000002",
			expTc:    ctx(1, 2, 1),
			expNoErr: true,
		},
		{
			name:     "multiple sampled uses first 1",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000002;Sampled=1;Sampled=0",
			expTc:    ctx(1, 2, 1),
			expNoErr: true,
		},
		{
			name:     "multiple sampled uses first 0",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000002;Sampled=0;Sampled=1",
			expTc:    ctx(1, 2, 0),
			expNoErr: true,
		},
		{
			name:     "sampled empty",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000002;Sampled=",
			expTc:    ctx(1, 2, 0),
			expNoErr: true,
		},
		{
			name:     "sampled empty then sampled found",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000002;Sampled=;Sampled=1",
			expTc:    ctx(1, 2, 1),
			expNoErr: true,
		},
		{
			name:     "with lineage",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;Lineage=a87bd80c:1|68fd508a:5|c512fbe3:2",
			expTc:    ctx(1, 1, 0),
			expNoErr: true,
		},
		{
			name:     "root too long",
			value:    "Root=1-00000000-0000000000000000000000010000;Parent=0000000000000001",
			expTc:    ctx(65536, 1, 0),
			expNoErr: true,
		},
		{
			name:     "parent too long",
			value:    "Root=1-00000000-000000000000000000000001;Parent=00000000000000010000",
			expTc:    ctx(1, 65536, 0),
			expNoErr: true,
		},
		{
			name:     "invalid root chars",
			value:    "Root=1-00000000-00000000000000000traceID;Parent=0000000000000000",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "invalid parent chars",
			value:    "Root=1-00000000-000000000000000000000000;Parent=0000000000spanID",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "invalid root and parent chars",
			value:    "Root=1-00000000-00000000000000000traceID;Parent=0000000000spanID",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "large trace-id",
			value:    "Root=1-5759e988-bd862e3fe1be46a994272793;Parent=53995c3f42cd8ad8",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "non-zero epoch",
			value:    "Root=1-5759e988-00000000e1be46a994272793;Parent=53995c3f42cd8ad8",
			expTc:    ctx(16266516598257821587, 6023947403358210776, 0),
			expNoErr: true,
		},
		{
			name:     "unknown key/value",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;key=value",
			expTc:    ctx(1, 1, 0),
			expNoErr: true,
		},
		{
			name:     "key no value",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;key=",
			expTc:    ctx(1, 1, 0),
			expNoErr: true,
		},
		{
			name:     "value no key",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;=value",
			expTc:    ctx(1, 1, 0),
			expNoErr: true,
		},
		{
			name:     "extra chars suffix",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;value",
			expTc:    ctx(1, 1, 0),
			expNoErr: true,
		},
		{
			name:     "root key no root value",
			value:    "Root=;Parent=0000000000000001",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "parent key no parent value",
			value:    "Root=1-00000000-000000000000000000000001;Parent=",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "bad trace id",
			value:    "Root=1-00000000-000000000000000000000001purple;Parent=0000000000000002;Sampled=1",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "bad parent id",
			value:    "Root=1-00000000-000000000000000000000001;Parent=0000000000000002purple;Sampled=1",
			expTc:    nil,
			expNoErr: false,
		},
		{
			name:     "zero value trace and parent id",
			value:    "Root=1-00000000-000000000000000000000000;Parent=0000000000000000;Sampled=1",
			expTc:    nil,
			expNoErr: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			ctx, err := extractTraceContextfromAWSTraceHeader(tc.value)
			t.Logf("extractTraceContextfromAWSTraceHeader returned TraceContext=%#v error=%#v", ctx, err)
			assert.Equal(tc.expTc, ctx)
			assert.Equal(tc.expNoErr, err == nil)
		})
	}
}

func TestSqsMessageCarrier(t *testing.T) {
	testcases := []struct {
		name   string
		event  events.SQSMessage
		expMap map[string]string
		expErr error
	}{
		{
			name:   "datadog-map",
			event:  eventSqsMessage(headersNone, headersAll, headersNone),
			expMap: headersMapAll,
			expErr: nil,
		},
		{
			name:   "datadog-map",
			event:  eventSqsMessage(headersAll, headersNone, headersNone),
			expMap: headersMapAll,
			expErr: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tm, err := sqsMessageCarrier(tc.event)
			t.Logf("sqsMessageCarrier returned TextMapReader=%#v error=%#v", tm, err)
			assert.Equal(t, tc.expErr == nil, err == nil)
			if err != nil {
				assert.Equal(t, tc.expErr.Error(), err.Error())
			}
			assert.Equal(t, tc.expMap, getMapFromCarrier(tm))
		})
	}
}

func TestRawPayloadCarrier(t *testing.T) {
	testcases := []struct {
		name   string
		event  []byte
		expMap map[string]string
		expErr error
	}{
		{
			name:   "empty-string",
			event:  []byte(headersNone),
			expMap: headersMapNone,
			expErr: errors.New("Could not unmarshal the invocation event payload"),
		},
		{
			name:   "empty-map",
			event:  []byte(headersEmpty),
			expMap: headersMapEmpty,
			expErr: nil,
		},
		{
			name:   "no-headers-key",
			event:  []byte(`{"hello":"world"}`),
			expMap: headersMapEmpty,
			expErr: nil,
		},
		{
			name:   "not-map-type",
			event:  []byte("[]"),
			expMap: headersMapNone,
			expErr: errors.New("Could not unmarshal the invocation event payload"),
		},
		{
			name:   "toplevel-headers-all",
			event:  []byte(headersAll),
			expMap: headersMapEmpty,
			expErr: nil,
		},
		{
			name:   "keyed-headers-all",
			event:  []byte(`{"headers":` + headersAll + `}`),
			expMap: headersMapAll,
			expErr: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tm, err := rawPayloadCarrier(tc.event)
			t.Logf("rawPayloadCarrier returned TextMapReader=%#v error=%#v", tm, err)
			assert.Equal(t, tc.expErr != nil, err != nil)
			if tc.expErr != nil && err != nil {
				assert.Equal(t, tc.expErr.Error(), err.Error())
			}
			assert.Equal(t, tc.expMap, getMapFromCarrier(tm))
		})
	}
}

func TestHeadersCarrier(t *testing.T) {
	testcases := []struct {
		name   string
		event  map[string]string
		expMap map[string]string
		expErr error
	}{
		{
			name:   "nil-map",
			event:  headersMapNone,
			expMap: headersMapEmpty,
			expErr: nil,
		},
		{
			name:   "empty-map",
			event:  headersMapEmpty,
			expMap: headersMapEmpty,
			expErr: nil,
		},
		{
			name:   "headers-all",
			event:  headersMapAll,
			expMap: headersMapAll,
			expErr: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tm, err := headersCarrier(tc.event)
			t.Logf("rawPayloadCarrier returned TextMapReader=%#v error=%#v", tm, err)
			assert.Equal(t, tc.expErr != nil, err != nil)
			if tc.expErr != nil && err != nil {
				assert.Equal(t, tc.expErr.Error(), err.Error())
			}
			assert.Equal(t, tc.expMap, getMapFromCarrier(tm))
		})
	}
}
