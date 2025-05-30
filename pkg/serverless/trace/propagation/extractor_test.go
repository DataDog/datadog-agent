// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package propagation

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"testing"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
)

type uintItem struct {
	asUint uint64
	asStr  string
}
type intItem struct {
	asInt sampler.SamplingPriority
	asStr string
}
type context struct {
	trace    uintItem
	span     uintItem
	priority intItem
}

var (
	dd = context{
		trace:    uintItem{1, "0000000000000000001"},
		span:     uintItem{2, "0000000000000000002"},
		priority: intItem{2, "2"},
	}
	w3c = context{
		trace:    uintItem{3, "0000000000000003"},
		span:     uintItem{4, "0000000000000004"},
		priority: intItem{3, "3"},
	}
	ddx = context{
		trace:    uintItem{5, "0000000000000000005"},
		span:     uintItem{6, "0000000000000000006"},
		priority: intItem{0, "0"},
	}
	x = context{
		trace:    uintItem{7, "0000000000000000007"},
		span:     uintItem{8, "0000000000000000008"},
		priority: intItem{0, "0"},
	}
)

var (
	ddTraceContext = &TraceContext{
		TraceID:          dd.trace.asUint,
		ParentID:         dd.span.asUint,
		SamplingPriority: dd.priority.asInt,
	}
	w3cTraceContext = &TraceContext{
		TraceID:          w3c.trace.asUint,
		ParentID:         w3c.span.asUint,
		SamplingPriority: w3c.priority.asInt,
	}
	ddxTraceContext = &TraceContext{
		TraceID:          ddx.trace.asUint,
		ParentID:         ddx.span.asUint,
		SamplingPriority: ddx.priority.asInt,
	}
)

var (
	headersMapNone  = map[string]string(nil)
	headersMapEmpty = map[string]string{}
	headersMapAll   = map[string]string{
		"x-datadog-trace-id":               dd.trace.asStr,
		"x-datadog-parent-id":              dd.span.asStr,
		"x-datadog-sampling-priority":      dd.priority.asStr,
		"x-datadog-tags":                   "_dd.p.dm=-0",
		"x-datadog-span-id":                "1234",
		"x-datadog-invocation-error":       "true",
		"x-datadog-invocation-error-msg":   "oops",
		"x-datadog-invocation-error-type":  "RuntimeError",
		"x-datadog-invocation-error-stack": "pancakes",
		"traceparent":                      "00-0000000000000000" + w3c.trace.asStr + "-" + w3c.span.asStr + "-01",
		"tracestate":                       "dd=s:" + w3c.priority.asStr + ";t.dm:-0",
	}
	headersMapDD = map[string]string{
		"x-datadog-trace-id":               dd.trace.asStr,
		"x-datadog-parent-id":              dd.span.asStr,
		"x-datadog-sampling-priority":      dd.priority.asStr,
		"x-datadog-tags":                   "_dd.p.dm=-0",
		"x-datadog-span-id":                "1234",
		"x-datadog-invocation-error":       "true",
		"x-datadog-invocation-error-msg":   "oops",
		"x-datadog-invocation-error-type":  "RuntimeError",
		"x-datadog-invocation-error-stack": "pancakes",
	}
	headersMapW3C = map[string]string{
		"traceparent": "00-0000000000000000" + w3c.trace.asStr + "-" + w3c.span.asStr + "-01",
		"tracestate":  "dd=s:" + w3c.priority.asStr + ";t.dm:-0",
	}

	headersNone  = ""
	headersEmpty = "{}"
	headersAll   = func() string {
		hdr, _ := json.Marshal(headersMapAll)
		return string(hdr)
	}()
	headersDD = func() string {
		hdr, _ := json.Marshal(headersMapDD)
		return string(hdr)
	}()
	headersW3C = func() string {
		hdr, _ := json.Marshal(headersMapW3C)
		return string(hdr)
	}()
	headersDdXray = "Root=1-00000000-00000000" + ddx.trace.asStr + ";Parent=" + ddx.span.asStr
	headersXray   = "Root=1-12345678-12345678" + x.trace.asStr + ";Parent=" + x.span.asStr

	eventSqsMessage = func(sqsHdrs, snsHdrs, awsHdr string) events.SQSMessage {
		e := events.SQSMessage{}
		if sqsHdrs != "" {
			e.MessageAttributes = map[string]events.SQSMessageAttribute{
				"_datadog": {
					DataType:    "String",
					StringValue: aws.String(sqsHdrs),
				},
			}
		}
		if snsHdrs != "" {
			e.Body = `{
				"MessageAttributes": {
					"_datadog": {
						"Type": "Binary",
						"Value": "` + base64.StdEncoding.EncodeToString([]byte(snsHdrs)) + `"
					}
				}
			}`
		}
		if awsHdr != "" {
			e.Attributes = map[string]string{
				awsTraceHeader: awsHdr,
			}
		}
		return e
	}

	eventSnsEntity = func(binHdrs, strHdrs string) events.SNSEntity {
		e := events.SNSEntity{}
		if len(binHdrs) > 0 && len(strHdrs) == 0 {
			e.MessageAttributes = map[string]interface{}{
				"_datadog": map[string]interface{}{
					"Type":  "Binary",
					"Value": base64.StdEncoding.EncodeToString([]byte(binHdrs)),
				},
			}
		} else if len(binHdrs) == 0 && len(strHdrs) > 0 {
			e.MessageAttributes = map[string]interface{}{
				"_datadog": map[string]interface{}{
					"Type":  "String",
					"Value": strHdrs,
				},
			}
		} else if len(binHdrs) > 0 && len(strHdrs) > 0 {
			panic("expecting one of binHdrs or strHdrs, not both")
		}
		return e
	}
)

func toMultiValueHeaders(headers map[string]string) map[string][]string {
	mvh := make(map[string][]string)
	for k, v := range headers {
		mvh[k] = []string{v}
	}
	return mvh
}

func TestNilPropagator(t *testing.T) {
	var extractor Extractor
	tc, err := extractor.Extract([]byte(`{"headers":` + headersAll + `}`))
	t.Logf("Extract returned TraceContext=%#v error=%#v", tc, err)
	assert.Nil(t, err)
	assert.Equal(t, ddTraceContext, tc)
}

func TestExtractorExtract(t *testing.T) {
	testcases := []struct {
		name     string
		events   []interface{}
		expCtx   *TraceContext
		expNoErr bool
	}{
		{
			name: "unsupported-event",
			events: []interface{}{
				"hello world",
			},
			expCtx:   nil,
			expNoErr: false,
		},
		{
			name:     "no-events",
			events:   []interface{}{},
			expCtx:   nil,
			expNoErr: false,
		},

		// []byte
		{
			name: "bytes",
			events: []interface{}{
				[]byte(`{"headers":` + headersAll + `}`),
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// events.SQSEvent
		{
			name: "sqs-event-no-records",
			events: []interface{}{
				events.SQSEvent{
					Records: []events.SQSMessage{},
				},
			},
			expCtx:   nil,
			expNoErr: false,
		},
		{
			name: "sqs-event-uses-first-record",
			events: []interface{}{
				events.SQSEvent{
					Records: []events.SQSMessage{
						// Uses the first message only
						eventSqsMessage(headersDD, headersNone, headersNone),
						eventSqsMessage(headersW3C, headersNone, headersNone),
					},
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "sqs-event-uses-first-record-empty",
			events: []interface{}{
				events.SQSEvent{
					Records: []events.SQSMessage{
						// Uses the first message only
						eventSqsMessage(headersNone, headersNone, headersNone),
						eventSqsMessage(headersW3C, headersNone, headersNone),
					},
				},
			},
			expCtx:   nil,
			expNoErr: false,
		},

		// events.SQSMessage
		{
			name: "unable-to-get-carrier",
			events: []interface{}{
				events.SQSMessage{Body: ""},
			},
			expCtx:   nil,
			expNoErr: false,
		},
		{
			name: "extraction-error",
			events: []interface{}{
				eventSqsMessage(headersEmpty, headersNone, headersNone),
			},
			expCtx:   nil,
			expNoErr: false,
		},
		{
			name: "extract-from-sqs",
			events: []interface{}{
				eventSqsMessage(headersAll, headersNone, headersNone),
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "extract-from-snssqs",
			events: []interface{}{
				eventSqsMessage(headersNone, headersAll, headersNone),
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "extract-from-sqs-attrs",
			events: []interface{}{
				eventSqsMessage(headersW3C, headersDD, headersDdXray),
			},
			expCtx:   ddxTraceContext,
			expNoErr: true,
		},
		{
			name: "sqs-precidence-attrs",
			events: []interface{}{
				eventSqsMessage(headersW3C, headersDD, headersDdXray),
			},
			expCtx:   ddxTraceContext,
			expNoErr: true,
		},
		{
			name: "sqs-precidence-sqs",
			events: []interface{}{
				eventSqsMessage(headersW3C, headersDD, headersXray),
			},
			expCtx:   w3cTraceContext,
			expNoErr: true,
		},
		{
			name: "sqs-precidence-snssqs",
			events: []interface{}{
				eventSqsMessage(headersNone, headersDD, headersXray),
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// events.SNSEvent
		{
			name: "sns-event-no-records",
			events: []interface{}{
				events.SNSEvent{
					Records: []events.SNSEventRecord{},
				},
			},
			expCtx:   nil,
			expNoErr: false,
		},
		{
			name: "sns-event-uses-first-record",
			events: []interface{}{
				events.SNSEvent{
					Records: []events.SNSEventRecord{
						// Uses the first message only
						{SNS: eventSnsEntity(headersDD, headersNone)},
						{SNS: eventSnsEntity(headersW3C, headersNone)},
					},
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "sqs-event-uses-first-record-empty",
			events: []interface{}{
				events.SNSEvent{
					Records: []events.SNSEventRecord{
						// Uses the first message only
						{SNS: eventSnsEntity(headersNone, headersNone)},
						{SNS: eventSnsEntity(headersW3C, headersNone)},
					},
				},
			},
			expCtx:   nil,
			expNoErr: false,
		},

		// events.SNSEntity
		{
			name: "unable-to-get-carrier",
			events: []interface{}{
				events.SNSEntity{},
			},
			expCtx:   nil,
			expNoErr: false,
		},
		{
			name: "extraction-error",
			events: []interface{}{
				events.SNSEvent{
					Records: []events.SNSEventRecord{
						{SNS: eventSnsEntity(headersNone, headersNone)},
					},
				},
			},
			expCtx:   nil,
			expNoErr: false,
		},
		{
			name: "extract-binary",
			events: []interface{}{
				events.SNSEvent{
					Records: []events.SNSEventRecord{
						{SNS: eventSnsEntity(headersAll, headersNone)},
					},
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "extract-string",
			events: []interface{}{
				events.SNSEvent{
					Records: []events.SNSEventRecord{
						{SNS: eventSnsEntity(headersNone, headersAll)},
					},
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// events.EventBridgeEvent
		{
			name: "eventbridge-event-empty",
			events: []interface{}{
				events.EventBridgeEvent{},
			},
			expCtx:   nil,
			expNoErr: false,
		},
		{
			name: "eventbridge-event-with-dd-headers",
			events: []interface{}{
				events.EventBridgeEvent{
					Detail: struct {
						TraceContext map[string]string `json:"_datadog"`
					}{
						TraceContext: headersMapDD,
					},
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "eventbridge-event-with-all-headers",
			events: []interface{}{
				events.EventBridgeEvent{
					Detail: struct {
						TraceContext map[string]string `json:"_datadog"`
					}{
						TraceContext: headersMapAll,
					},
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "eventbridge-event-with-w3c-headers",
			events: []interface{}{
				events.EventBridgeEvent{
					Detail: struct {
						TraceContext map[string]string `json:"_datadog"`
					}{
						TraceContext: headersMapW3C,
					},
				},
			},
			expCtx:   w3cTraceContext,
			expNoErr: true,
		},
		{
			name: "eventbridge-event-without-trace-context",
			events: []interface{}{
				events.EventBridgeEvent{
					Detail: struct {
						TraceContext map[string]string `json:"_datadog"`
					}{
						TraceContext: map[string]string{},
					},
				},
			},
			expCtx:   nil,
			expNoErr: false,
		},

		// events.APIGatewayProxyRequest:
		{
			name: "APIGatewayProxyRequest",
			events: []interface{}{
				events.APIGatewayProxyRequest{
					Headers: headersMapAll,
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// events.APIGatewayV2HTTPRequest:
		{
			name: "APIGatewayV2HTTPRequest",
			events: []interface{}{
				events.APIGatewayV2HTTPRequest{
					Headers: headersMapAll,
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// events.APIGatewayWebsocketProxyRequest:
		{
			name: "APIGatewayWebsocketProxyRequest",
			events: []interface{}{
				events.APIGatewayWebsocketProxyRequest{
					Headers: headersMapAll,
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// events.APIGatewayCustomAuthorizerRequestTypeRequest:
		{
			name: "APIGatewayCustomAuthorizerRequestTypeRequest",
			events: []interface{}{
				events.APIGatewayCustomAuthorizerRequestTypeRequest{
					Headers: headersMapAll,
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// events.ALBTargetGroupRequest:
		{
			name: "ALBTargetGroupRequest",
			events: []interface{}{
				events.ALBTargetGroupRequest{
					Headers: headersMapAll,
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "ALBTargetGroupRequestMultiValueHeaders",
			events: []interface{}{
				events.ALBTargetGroupRequest{
					MultiValueHeaders: toMultiValueHeaders(headersMapAll),
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// events.LambdaFunctionURLRequest:
		{
			name: "LambdaFunctionURLRequest",
			events: []interface{}{
				events.LambdaFunctionURLRequest{
					Headers: headersMapAll,
				},
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},

		// multiple events
		{
			name: "multiple-events-1",
			events: []interface{}{
				[]byte(`{}`),
				[]byte("hello-world"),
				eventSqsMessage(headersAll, headersNone, headersNone),
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "multiple-events-2",
			events: []interface{}{
				[]byte(`{}`),
				[]byte("hello-world"),
				eventSqsMessage(headersDD, headersNone, headersNone),
				eventSqsMessage(headersAll, headersNone, headersNone),
			},
			expCtx:   ddTraceContext,
			expNoErr: true,
		},
		{
			name: "multiple-events-3",
			events: []interface{}{
				[]byte(`{}`),
				[]byte("hello-world"),
			},
			expCtx:   nil,
			expNoErr: false,
		},

		// Step Functions events
		{
			name: "step function event",
			events: []interface{}{
				events.StepFunctionPayload{
					Execution: struct {
						ID           string
						RedriveCount uint16
					}{
						ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
						RedriveCount: 0,
					},
					State: struct {
						Name        string
						EnteredTime string
						RetryCount  uint16
					}{
						Name:        "agocsTest1",
						EnteredTime: "2024-07-30T20:46:20.824Z",
						RetryCount:  0,
					},
				},
			},
			expCtx: &TraceContext{
				TraceID:           5377636026938777059,
				TraceIDUpper64Hex: "6fb5c3a05c73dbfe",
				ParentID:          8947638978974359093,
				SamplingPriority:  1,
			},
			expNoErr: true,
		},
		{
			name: "nested step function event",
			events: []interface{}{
				events.NestedStepFunctionPayload{
					Payload: events.StepFunctionPayload{
						Execution: struct {
							ID           string
							RedriveCount uint16
						}{
							ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
							RedriveCount: 0,
						},
						State: struct {
							Name        string
							EnteredTime string
							RetryCount  uint16
						}{
							Name:        "agocsTest1",
							EnteredTime: "2024-07-30T20:46:20.824Z",
							RetryCount:  0,
						},
					},
					RootExecutionID:   "arn:aws:states:sa-east-1:425362996713:execution:invokeJavaLambda:4875aba4-ae31-4a4c-bf8a-63e9eee31dad",
					ServerlessVersion: "v1",
				},
			},
			expCtx: &TraceContext{
				TraceID:           1322229001489018110,
				TraceIDUpper64Hex: "579d19b3ee216ee9",
				ParentID:          8947638978974359093,
				SamplingPriority:  1,
			},
			expNoErr: true,
		},
		{
			name: "lambda root step function event",
			events: []interface{}{
				events.LambdaRootStepFunctionPayload{
					Payload: events.StepFunctionPayload{
						Execution: struct {
							ID           string
							RedriveCount uint16
						}{
							ID:           "arn:aws:states:us-east-1:425362996713:execution:agocsTestSF:aa6c9316-713a-41d4-9c30-61131716744f",
							RedriveCount: 0,
						},
						State: struct {
							Name        string
							EnteredTime string
							RetryCount  uint16
						}{
							Name:        "agocsTest1",
							EnteredTime: "2024-07-30T20:46:20.824Z",
							RetryCount:  0,
						},
					},
					TraceID:           "5821803790426892636",
					TraceTags:         "_dd.p.dm=-0,_dd.p.tid=672a7cb100000000",
					ServerlessVersion: "v1",
				},
			},
			expCtx: &TraceContext{
				TraceID:           5821803790426892636,
				TraceIDUpper64Hex: "672a7cb100000000",
				ParentID:          8947638978974359093,
				SamplingPriority:  1,
			},
			expNoErr: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			extractor := Extractor{}
			ctx, err := extractor.Extract(tc.events...)
			t.Logf("Extract returned TraceContext=%#v error=%#v", ctx, err)
			assert.Equal(t, tc.expNoErr, err == nil)
			assert.Equal(t, tc.expCtx, ctx)
		})
	}
}

func TestExtractorExtractPayloadJson(t *testing.T) {
	testcases := []struct {
		filename string
		eventTyp string
		expCtx   *TraceContext
	}{
		{
			filename: "api-gateway.json",
			eventTyp: "APIGatewayProxyRequest",
			expCtx: &TraceContext{
				TraceID:          12345,
				ParentID:         67890,
				SamplingPriority: 2,
			},
		},
		{
			filename: "sns-batch.json",
			eventTyp: "SNSEvent",
			expCtx: &TraceContext{
				TraceID:          4948377316357291421,
				ParentID:         6746998015037429512,
				SamplingPriority: 1,
			},
		},
		{
			filename: "sns.json",
			eventTyp: "SNSEvent",
			expCtx: &TraceContext{
				TraceID:          4948377316357291421,
				ParentID:         6746998015037429512,
				SamplingPriority: 1,
			},
		},
		{
			filename: "snssqs.json",
			eventTyp: "SQSEvent",
			expCtx: &TraceContext{
				TraceID:          1728904347387697031,
				ParentID:         353722510835624345,
				SamplingPriority: 1,
			},
		},
		{
			filename: "sqs-aws-header.json",
			eventTyp: "SQSEvent",
			expCtx: &TraceContext{
				TraceID:          12297829382473034410,
				ParentID:         13527612320720337851,
				SamplingPriority: 1,
			},
		},
		{
			filename: "sqs-batch.json",
			eventTyp: "SQSEvent",
			expCtx: &TraceContext{
				TraceID:          2684756524522091840,
				ParentID:         7431398482019833808,
				SamplingPriority: 1,
			},
		},
		{
			filename: "sqs.json",
			eventTyp: "SQSEvent",
			expCtx: &TraceContext{
				TraceID:          2684756524522091840,
				ParentID:         7431398482019833808,
				SamplingPriority: 1,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.filename, func(t *testing.T) {
			body, err := os.ReadFile("../testdata/event_samples/" + tc.filename)
			assert.NoError(t, err)

			var ev interface{}
			switch tc.eventTyp {
			case "APIGatewayProxyRequest":
				var event events.APIGatewayProxyRequest
				err = json.Unmarshal(body, &event)
				assert.NoError(t, err)
				ev = event
			case "SNSEvent":
				var event events.SNSEvent
				err = json.Unmarshal(body, &event)
				assert.NoError(t, err)
				ev = event
			case "SQSEvent":
				var event events.SQSEvent
				err = json.Unmarshal(body, &event)
				assert.NoError(t, err)
				ev = event
			default:
				t.Fatalf("bad type: %s", tc.eventTyp)
			}

			extractor := Extractor{}
			ctx, err := extractor.Extract(ev)
			t.Logf("Extract returned TraceContext=%#v error=%#v", ctx, err)
			assert.NoError(t, err)
			assert.Equal(t, tc.expCtx, ctx)
		})
	}
}

func TestPropagationStyle(t *testing.T) {
	testcases := []struct {
		name       string
		propType   string
		hdrs       string
		expTraceID uint64
	}{
		{
			name:       "no-type-headers-all",
			propType:   "",
			hdrs:       headersAll,
			expTraceID: dd.trace.asUint,
		},
		{
			name:       "datadog-type-headers-all",
			propType:   "datadog",
			hdrs:       headersAll,
			expTraceID: dd.trace.asUint,
		},
		{
			name:       "tracecontet-type-headers-all",
			propType:   "tracecontext",
			hdrs:       headersAll,
			expTraceID: w3c.trace.asUint,
		},
		{
			name:       "datadog,tracecontext-type-headers-all",
			propType:   "datadog,tracecontext",
			hdrs:       headersAll,
			expTraceID: dd.trace.asUint,
		},
		{
			name:       "tracecontext,datadog-type-headers-all",
			propType:   "tracecontext,datadog",
			hdrs:       headersAll,
			expTraceID: w3c.trace.asUint,
		},
		{
			name:       "datadog-type-headers-w3c",
			propType:   "datadog",
			hdrs:       headersW3C,
			expTraceID: 0,
		},
		{
			name:       "tracecontet-type-headers-dd",
			propType:   "tracecontext",
			hdrs:       headersDD,
			expTraceID: 0,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DD_TRACE_PROPAGATION_STYLE", tc.propType)
			extractor := Extractor{}
			event := eventSqsMessage(tc.hdrs, headersNone, headersNone)
			ctx, err := extractor.Extract(event)
			t.Logf("Extract returned TraceContext=%#v error=%#v", ctx, err)
			if tc.expTraceID == 0 {
				assert.NotNil(t, err)
				assert.Nil(t, ctx)
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tc.expTraceID, ctx.TraceID)
			}
		})
	}
}

func TestExtractorExtractFromLayer(t *testing.T) {
	convertMapToHeader := func(m map[string]string) http.Header {
		hdr := http.Header{}
		for k, v := range m {
			hdr.Set(k, v)
		}
		return hdr
	}
	allHeadersExcept := func(except string) http.Header {
		hdr := http.Header{}
		for k, v := range headersMapAll {
			if k == except {
				continue
			}
			hdr.Set(k, v)
		}
		return hdr
	}

	testcases := []struct {
		name     string
		propType string
		hdr      http.Header
		expCtx   *TraceContextExtended
	}{
		{
			name:     "empty-headers",
			propType: "datadog",
			hdr:      convertMapToHeader(headersMapEmpty),
			expCtx:   new(TraceContextExtended),
		},
		{
			name:     "missing-trace-id",
			propType: "datadog",
			hdr:      allHeadersExcept(ddTraceIDHeader),
			expCtx: &TraceContextExtended{
				TraceContext:    nil,
				SpanID:          1234,
				InvocationError: true,
			},
		},
		{
			name:     "missing-parent-id",
			propType: "datadog",
			hdr:      allHeadersExcept(ddParentIDHeader),
			expCtx: &TraceContextExtended{
				TraceContext:    nil,
				SpanID:          1234,
				InvocationError: true,
			},
		},
		{
			name:     "missing-sampling-priority",
			propType: "datadog",
			hdr:      allHeadersExcept(ddSamplingPriorityHeader),
			expCtx: &TraceContextExtended{
				TraceContext: &TraceContext{
					TraceID:          dd.trace.asUint,
					ParentID:         dd.span.asUint,
					SamplingPriority: sampler.PriorityNone,
				},
				SpanID:          1234,
				InvocationError: true,
			},
		},
		{
			name:     "missing-span-id",
			propType: "datadog",
			hdr:      allHeadersExcept(ddSpanIDHeader),
			expCtx: &TraceContextExtended{
				TraceContext:    ddTraceContext,
				SpanID:          0,
				InvocationError: true,
			},
		},
		{
			name:     "missing-invocation-error",
			propType: "datadog",
			hdr:      allHeadersExcept(ddInvocationErrorHeader),
			expCtx: &TraceContextExtended{
				TraceContext:    ddTraceContext,
				SpanID:          1234,
				InvocationError: false,
			},
		},
		{
			name:     "dd-hdrs-datadog-style",
			propType: "datadog",
			hdr:      convertMapToHeader(headersMapDD),
			expCtx: &TraceContextExtended{
				TraceContext:    ddTraceContext,
				SpanID:          1234,
				InvocationError: true,
			},
		},
		{
			name:     "w3c-hdrs-datadog-style",
			propType: "datadog",
			hdr:      convertMapToHeader(headersMapW3C),
			expCtx:   new(TraceContextExtended),
		},
		{
			name:     "all-hdrs-datadog-style",
			propType: "datadog",
			hdr:      convertMapToHeader(headersMapAll),
			expCtx: &TraceContextExtended{
				TraceContext:    ddTraceContext,
				SpanID:          1234,
				InvocationError: true,
			},
		},
		{
			name:     "dd-hdrs-tracecontext-style",
			propType: "tracecontext",
			hdr:      convertMapToHeader(headersMapDD),
			expCtx: &TraceContextExtended{
				TraceContext:    ddTraceContext,
				SpanID:          1234,
				InvocationError: true,
			},
		},
		{
			name:     "w3c-hdrs-tracecontext-style",
			propType: "tracecontext",
			hdr:      convertMapToHeader(headersMapW3C),
			expCtx:   new(TraceContextExtended),
		},
		{
			name:     "all-hdrs-tracecontext-style",
			propType: "tracecontext",
			hdr:      convertMapToHeader(headersMapAll),
			expCtx: &TraceContextExtended{
				TraceContext:    ddTraceContext,
				SpanID:          1234,
				InvocationError: true,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DD_TRACE_PROPAGATION_STYLE", tc.propType)
			ctx := Extractor{}.ExtractFromLayer(tc.hdr)
			t.Logf("ExtractFromLayer returned TraceContextExtended=%#v", ctx)
			assert.Equal(t, tc.expCtx, ctx)
		})
	}
}

func TestInjectToLayer(t *testing.T) {
	testcases := []struct {
		name     string
		propType string
		ctx      *TraceContext
		expHdr   http.Header
	}{
		{
			name:     "nil-trace-context",
			propType: "datadog",
			ctx:      nil,
			expHdr:   http.Header{},
		},
		{
			name:     "empty-context",
			propType: "datadog",
			ctx:      new(TraceContext),
			expHdr: http.Header{
				"X-Datadog-Trace-Id":          []string{"0"},
				"X-Datadog-Sampling-Priority": []string{"0"},
			},
		},
		{
			name:     "dd-context-datadog-style",
			propType: "datadog",
			ctx:      ddTraceContext,
			expHdr: http.Header{
				"X-Datadog-Trace-Id":          []string{"1"},
				"X-Datadog-Sampling-Priority": []string{"2"},
			},
		},
		{
			name:     "dd-context-tracecontext-style",
			propType: "tracecontext",
			ctx:      ddTraceContext,
			expHdr: http.Header{
				"X-Datadog-Trace-Id":          []string{"1"},
				"X-Datadog-Sampling-Priority": []string{"2"},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DD_TRACE_PROPAGATION_STYLE", tc.propType)
			hdr := http.Header{}
			Extractor{}.InjectToLayer(tc.ctx, hdr)
			t.Logf("InjectToLayer resulted http.Header=%#v", hdr)
			assert.Equal(t, tc.expHdr, hdr)
		})
	}
}

type mockSpanContext struct{}

func (m mockSpanContext) SpanID() uint64 {
	return 2
}
func (m mockSpanContext) TraceID() uint64 {
	return 2
}
func (m mockSpanContext) ForeachBaggageItem(_ func(k, v string) bool) {}

type mockSpanContextWithSamplingPriority struct {
	mockSpanContext
	ok bool
}

func (m mockSpanContextWithSamplingPriority) SamplingPriority() (int, bool) {
	return 2, m.ok
}

func TestGetSamplingPriority(t *testing.T) {
	testcases := []struct {
		name  string
		ctx   ddtrace.SpanContext
		expPr sampler.SamplingPriority
	}{
		{
			name:  "nil-context",
			ctx:   nil,
			expPr: sampler.PriorityNone,
		},
		{
			name:  "no-sampling-priority-method",
			ctx:   mockSpanContext{},
			expPr: sampler.PriorityNone,
		},
		{
			name:  "sampling-priority-method-returns-not-ok",
			ctx:   mockSpanContextWithSamplingPriority{ok: false},
			expPr: sampler.PriorityNone,
		},
		{
			name:  "sampling-priority-method-returns-ok",
			ctx:   mockSpanContextWithSamplingPriority{ok: true},
			expPr: 2,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			priority := getSamplingPriority(tc.ctx)
			t.Logf("getSamplingPriority returned priority=%#v", priority)
			assert.Equal(t, tc.expPr, priority)
		})
	}
}

func TestConvertStrToUint64(t *testing.T) {
	testcases := []struct {
		val     string
		expUint uint64
		expErr  error
	}{
		{
			val:     "1234",
			expUint: 1234,
			expErr:  nil,
		},
		{
			val:     "invalid",
			expUint: 0,
			expErr:  errors.New("strconv.ParseUint: parsing \"invalid\": invalid syntax"),
		},
		{
			val:     "-1234",
			expUint: 0,
			expErr:  errors.New("strconv.ParseUint: parsing \"-1234\": invalid syntax"),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.val, func(t *testing.T) {
			value, err := convertStrToUint64(tc.val)
			assert.Equal(t, tc.expUint, value)
			assert.Equal(t, tc.expErr != nil, err != nil)
			if tc.expErr != nil && err != nil {
				assert.Equal(t, tc.expErr.Error(), err.Error())
			}
		})
	}
}

func TestEventBridgeCarrierWithW3CHeaders(t *testing.T) {
	const (
		testResourceName = "test-event-bus"
		testStartTime    = "1632150183123456789"
	)

	event := events.EventBridgeEvent{
		Detail: struct {
			TraceContext map[string]string `json:"_datadog"`
		}{
			TraceContext: map[string]string{
				"traceparent":             headersMapW3C["traceparent"],
				"tracestate":              headersMapW3C["tracestate"],
				"x-datadog-resource-name": testResourceName,
				"x-datadog-start-time":    testStartTime,
			},
		},
	}

	carrier, err := eventBridgeCarrier(event)
	assert.NoError(t, err)
	assert.NotNil(t, carrier)

	textMapCarrier, ok := carrier.(tracer.TextMapCarrier)
	assert.True(t, ok)

	assert.Equal(t, headersMapW3C["traceparent"], textMapCarrier["traceparent"])
	assert.Equal(t, headersMapW3C["tracestate"], textMapCarrier["tracestate"])
	assert.Equal(t, testResourceName, textMapCarrier["x-datadog-resource-name"])
	assert.Equal(t, testStartTime, textMapCarrier["x-datadog-start-time"])
}
