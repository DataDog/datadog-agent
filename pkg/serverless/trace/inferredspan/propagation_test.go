// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"encoding/base64"
	"strconv"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

func TestExtractContextFromSNSSQSEvent_ValidBinaryTraceData(t *testing.T) {
	mockSQSMessage := events.SQSMessage{
		Body: `{
            "Message": "mock message",
            "MessageAttributes": {
                "_datadog": {
                    "Type": "Binary",
                    "Value": "eyJ0cmFjZXBhcmVudCI6IjAwLTAwMDAwMDAwMDAwMDAwMDAxN2ZlNGQ1ODA0YWMxNzg3LTA0ZThhY2JmZGY2YWE5OTktMDEiLCJ0cmFjZXN0YXRlIjoiZGQ9czoxO3QuZG06LTEiLCJ4LWRhdGFkb2ctdHJhY2UtaWQiOiIxNzI4OTA0MzQ3Mzg3Njk3MDMxIiwieC1kYXRhZG9nLXBhcmVudC1pZCI6IjM1MzcyMjUxMDgzNTYyNDM0NSIsIngtZGF0YWRvZy1zYW1wbGluZy1wcmlvcml0eSI6IjEiLCJ4LWRhdGFkb2ctdGFncyI6Il9kZC5wLmRtPS0xIn0="
                }
            }
        }`,
	}

	rawTraceContext := extractTraceContextFromSNSSQSEvent(mockSQSMessage)

	assert.NotNil(t, rawTraceContext)
	assert.Equal(t, "1728904347387697031", rawTraceContext.TraceID)
	assert.Equal(t, "353722510835624345", rawTraceContext.ParentID)
}

func TestExtractContextFromSNSSQSEvent_InvalidBinaryTraceData(t *testing.T) {
	// In this case, the binary payload (Value) is not a valid base64 encoding of a TraceHeader JSON object
	mockSQSMessage := events.SQSMessage{
		Body: `{
            "Message": "mock message",
            "MessageAttributes": {
                "_datadog": {
                    "Type": "Binary",
                    "Value": "invalid binary data"
                }
            }
        }`,
	}

	rawTraceContext := extractTraceContextFromSNSSQSEvent(mockSQSMessage)

	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromSNSSQSEvent_InvalidJsonBody(t *testing.T) {
	mockSQSMessage := events.SQSMessage{
		Body: `invalid json`,
	}

	rawTraceContext := extractTraceContextFromSNSSQSEvent(mockSQSMessage)
	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromSNSSQSEvent_NoDatadogTraceContext(t *testing.T) {
	mockSQSMessage := events.SQSMessage{
		Body: `{
            "Message": "mock message",
            "MessageAttributes": {}
        }`,
	}

	rawTraceContext := extractTraceContextFromSNSSQSEvent(mockSQSMessage)
	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromSQSEvent_NoDatadogTraceContext(t *testing.T) {
	stringValue := "mock message"
	mockSQSMessage := events.SQSMessageAttribute{
		StringValue: &stringValue,
		DataType:    "String",
	}

	rawTraceContext := extractTraceContextFromDatadogHeader(mockSQSMessage)
	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromSNSSQSEvent_UnsupportedDataType(t *testing.T) {
	mockSQSMessage := events.SQSMessage{
		Body: `{
            "Message": "mock message",
            "MessageAttributes": {
                "_datadog": {
                    "Type": "Unsupported",
                    "Value": "eyJ4LWRhdGFkb2ctdHJhY2UtaWQiOiAiMTIzNDU2Nzg5MCIsICJ4LWRhdGFkb2ctcGFyZW50LWlkIjogIjEyMzQ1Njc4OTAiLCAieC1kYXRhZG9nLXNhbXBsaW5nLXByaW9yaXR5IjogIjEuMCJ9"
                }
            }
        }`,
	}

	rawTraceContext := extractTraceContextFromSNSSQSEvent(mockSQSMessage)
	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromSNSSQSEvent_InvalidBase64(t *testing.T) {
	mockSQSMessage := events.SQSMessage{
		Body: `{
            "Message": "mock message",
            "MessageAttributes": {
                "_datadog": {
                    "Type": "Binary",
                    "Value": "invalid base64"
                }
            }
        }`,
	}

	rawTraceContext := extractTraceContextFromSNSSQSEvent(mockSQSMessage)
	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromPureSqsEvent_ValidStringTraceData(t *testing.T) {
	str := `{"x-datadog-trace-id": "3754030949214830614", "x-datadog-parent-id": "9807017789787771839", "x-datadog-sampling-priority": "1", "x-datadog-tags": "_dd.p.dm=-0", "traceparent": "00-00000000000000003418ff4233c5c016-881986b8523c93bf-01", "tracestate": "dd=s:1;t.dm:-0"}`
	mockSQSMessageAttribute := events.SQSMessageAttribute{
		DataType:    "String",
		StringValue: &str,
	}

	rawTraceContext := extractTraceContextFromDatadogHeader(mockSQSMessageAttribute)

	assert.NotNil(t, rawTraceContext)
	assert.Equal(t, "3754030949214830614", rawTraceContext.TraceID)
	assert.Equal(t, "9807017789787771839", rawTraceContext.ParentID)
}

func TestExtractContextFromPureSqsEvent_ValidBinaryTraceData(t *testing.T) {
	// SNS => SQS => Lambda with SQS's subscription to SNS has enabled RAW MESSAGE DELIVERY option
	str := `{"x-datadog-trace-id": "3754030949214830614", "x-datadog-parent-id": "9807017789787771839", "x-datadog-sampling-priority": "1", "x-datadog-tags": "_dd.p.dm=-0", "traceparent": "00-00000000000000003418ff4233c5c016-881986b8523c93bf-01", "tracestate": "dd=s:1;t.dm:-0"}`
	mockSQSMessageAttribute := events.SQSMessageAttribute{
		DataType:    "Binary",
		BinaryValue: []byte(str),
	}

	rawTraceContext := extractTraceContextFromDatadogHeader(mockSQSMessageAttribute)

	assert.NotNil(t, rawTraceContext)
	assert.Equal(t, "3754030949214830614", rawTraceContext.TraceID)
	assert.Equal(t, "9807017789787771839", rawTraceContext.ParentID)
}

func TestExtractContextFromPureSqsEvent_InvalidStringTraceData(t *testing.T) {
	// In this case, the string payload (StringValue) is not a valid TraceHeader JSON object
	mockSQSMessageAttribute := events.SQSMessageAttribute{
		DataType:    "String",
		StringValue: aws.String(`invalid string data`),
	}

	rawTraceContext := extractTraceContextFromDatadogHeader(mockSQSMessageAttribute)

	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromPureSqsEvent_InvalidBinaryTraceData(t *testing.T) {
	// This is a failure case because we expect base64 decoding already done at this point (by json.Unmarshal)
	str := "eyJ0cmFjZXBhcmVudCI6IjAwLTAwMDAwMDAwMDAwMDAwMDA1ZmExMmM3MDQ3Y2Y3OWQ3LTM2ZTg2OGRkODgwZjY5OTEtMDEiLCJ0cmFjZXN0YXRlIjoiZGQ9czoxO3QuZG06LTAiLCJ4LWRhdGFkb2ctdHJhY2UtaWQiOiI2ODkwODM3NzY1NjA2MzA4MzExIiwieC1kYXRhZG9nLXBhcmVudC1pZCI6IjM5NTY1Mjc1NzMzMjQ3NTMyOTciLCJ4LWRhdGFkb2ctc2FtcGxpbmctcHJpb3JpdHkiOiIxIiwieC1kYXRhZG9nLXRhZ3MiOiJfZGQucC5kbT0tMCJ9"
	mockSQSMessageAttribute := events.SQSMessageAttribute{
		DataType:    "Binary",
		BinaryValue: []byte(str),
	}

	rawTraceContext := extractTraceContextFromDatadogHeader(mockSQSMessageAttribute)

	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromPureSqsEvent_InvalidJson(t *testing.T) {
	mockSQSMessageAttribute := events.SQSMessageAttribute{
		DataType:    "String",
		StringValue: aws.String(`invalid json`),
	}

	rawTraceContext := extractTraceContextFromDatadogHeader(mockSQSMessageAttribute)
	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromPureSqsEvent_UnsupportedDataType(t *testing.T) {
	mockSQSMessageAttribute := events.SQSMessageAttribute{
		DataType: "Unsupported",
		StringValue: aws.String(`{
            "x-datadog-trace-id": "1234567890",
            "x-datadog-parent-id": "1234567890",
            "x-datadog-sampling-priority": "1.0"
        }`),
	}

	rawTraceContext := extractTraceContextFromDatadogHeader(mockSQSMessageAttribute)
	assert.Nil(t, rawTraceContext)
}
func TestConvertRawTraceContext_ValidInput(t *testing.T) {
	rawTrace := &rawTraceContext{
		TraceID:  "1234567890",
		ParentID: "1234567890",
	}

	convertedTraceContext := convertRawTraceContext(rawTrace)

	assert.Equal(t, uint64(1234567890), *convertedTraceContext.ParentID)
	assert.Equal(t, uint64(1234567890), *convertedTraceContext.TraceID)
}

func TestConvertRawTraceContext_InvalidInput(t *testing.T) {
	rawTrace := &rawTraceContext{
		TraceID:  "invalid",
		ParentID: "invalid",
	}

	convertedTraceContext := convertRawTraceContext(rawTrace)

	assert.Nil(t, convertedTraceContext)
}

func TestConvertRawTraceContext_DecimalBase(t *testing.T) {
	rawTrace := &rawTraceContext{
		TraceID:  "1234567890",
		ParentID: "1234567890",
		base:     10,
	}

	convertedTraceContext := convertRawTraceContext(rawTrace)

	assert.Equal(t, uint64(1234567890), *convertedTraceContext.ParentID)
	assert.Equal(t, uint64(1234567890), *convertedTraceContext.TraceID)
}

func TestConvertRawTraceContext_HexBase(t *testing.T) {
	rawTrace := &rawTraceContext{
		TraceID:  strconv.FormatInt(1234567890, 16),
		ParentID: strconv.FormatInt(1234567890, 16),
		base:     16,
	}

	convertedTraceContext := convertRawTraceContext(rawTrace)

	assert.Equal(t, uint64(1234567890), *convertedTraceContext.ParentID)
	assert.Equal(t, uint64(1234567890), *convertedTraceContext.TraceID)
}

func TestExtractTraceContextfromAWSTraceHeader(t *testing.T) {
	ctx := func(trace, parent string) *rawTraceContext {
		return &rawTraceContext{
			TraceID:  trace,
			ParentID: parent,
			base:     16,
		}
	}

	testcases := []struct {
		name   string
		value  string
		expect *rawTraceContext
	}{
		{
			name:   "empty string",
			value:  "",
			expect: nil,
		},
		{
			name:   "root but no parent",
			value:  "Root=1-00000000-000000000000000000000001",
			expect: ctx("0000000000000001", ""),
		},
		{
			name:   "parent but no root",
			value:  "Parent=0000000000000001",
			expect: nil,
		},
		{
			name:   "just root and parent",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000001",
			expect: ctx("0000000000000001", "0000000000000001"),
		},
		{
			name:   "trailing semi-colon",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;",
			expect: ctx("0000000000000001", "0000000000000001"),
		},
		{
			name:   "trailing semi-colons",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;;;",
			expect: ctx("0000000000000001", "0000000000000001"),
		},
		{
			name:   "parent first",
			value:  "Parent=0000000000000009;Root=1-00000000-000000000000000000000001",
			expect: ctx("0000000000000001", "0000000000000009"),
		},
		{
			name:   "two roots",
			value:  "Root=1-00000000-000000000000000000000005;Parent=0000000000000009;Root=1-00000000-000000000000000000000001",
			expect: ctx("0000000000000005", "0000000000000009"),
		},
		{
			name:   "two parents",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000009;Parent=0000000000000000",
			expect: ctx("0000000000000001", "0000000000000009"),
		},
		{
			name:   "sampled is ignored",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000002;Sampled=1",
			expect: ctx("0000000000000001", "0000000000000002"),
		},
		{
			name:   "with lineage",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;Lineage=a87bd80c:1|68fd508a:5|c512fbe3:2",
			expect: ctx("0000000000000001", "0000000000000001"),
		},
		{
			name:   "root too long",
			value:  "Root=1-00000000-0000000000000000000000010000;Parent=0000000000000001",
			expect: ctx("00000000000000010000", "0000000000000001"),
		},
		{
			name:   "parent too long",
			value:  "Root=1-00000000-000000000000000000000001;Parent=00000000000000010000",
			expect: ctx("0000000000000001", "00000000000000010000"),
		},
		{
			name:   "invalid root chars",
			value:  "Root=1-00000000-00000000000000000traceID;Parent=0000000000000000",
			expect: nil,
		},
		{
			name:   "invalid parent chars",
			value:  "Root=1-00000000-000000000000000000000000;Parent=0000000000spanID",
			expect: ctx("0000000000000000", "0000000000spanID"),
		},
		{
			name:   "invalid root and parent chars",
			value:  "Root=1-00000000-00000000000000000traceID;Parent=0000000000spanID",
			expect: nil,
		},
		{
			name:   "large trace-id",
			value:  "Root=1-5759e988-bd862e3fe1be46a994272793;Parent=53995c3f42cd8ad8",
			expect: nil,
		},
		{
			name:   "non-zero epoch",
			value:  "Root=1-5759e988-00000000e1be46a994272793;Parent=53995c3f42cd8ad8",
			expect: ctx("e1be46a994272793", "53995c3f42cd8ad8"),
		},
		{
			name:   "unknown key/value",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;key=value",
			expect: ctx("0000000000000001", "0000000000000001"),
		},
		{
			name:   "key no value",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;key=",
			expect: ctx("0000000000000001", "0000000000000001"),
		},
		{
			name:   "value no key",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;=value",
			expect: ctx("0000000000000001", "0000000000000001"),
		},
		{
			name:   "extra chars suffix",
			value:  "Root=1-00000000-000000000000000000000001;Parent=0000000000000001;value",
			expect: ctx("0000000000000001", "0000000000000001"),
		},
		{
			name:   "root key no root value",
			value:  "Root=;Parent=0000000000000001",
			expect: nil,
		},
		{
			name:   "parent key no parent value",
			value:  "Root=1-00000000-000000000000000000000001;Parent=",
			expect: ctx("0000000000000001", ""),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			actual := extractTraceContextfromAWSTraceHeader(tc.value)
			assert.Equal(tc.expect, actual)
		})
	}
}

func TestExtractTraceContext(t *testing.T) {
	_xrayDdogID := "0000000000000001"
	_xrayID := "0000000000000002"
	_datadogID := "0000000000000003"
	_snsID := "0000000000000004"

	makeEvent := func(ddxray, xray, ddog, sns bool) events.SQSMessage {
		assert.False(t, ddxray && xray)
		event := events.SQSMessage{
			Attributes:        make(map[string]string),
			MessageAttributes: make(map[string]events.SQSMessageAttribute),
		}
		if ddxray {
			event.Attributes[awsTraceHeader] = "Root=1-00000000-00000000" + _xrayDdogID + ";Parent=" + _xrayDdogID
		}
		if xray {
			event.Attributes[awsTraceHeader] = "Root=1-12345678-12345678" + _xrayID + ";Parent=" + _xrayID
		}
		if ddog {
			event.MessageAttributes[datadogHeader] = events.SQSMessageAttribute{
				DataType: "String",
				StringValue: aws.String(`{
					"x-datadog-trace-id": "` + _datadogID + `",
					"x-datadog-parent-id": "` + _datadogID + `"
				}`),
			}
		}
		if sns {
			encoded := base64.StdEncoding.EncodeToString([]byte(`{
				"x-datadog-trace-id": "` + _snsID + `",
				"x-datadog-parent-id": "` + _snsID + `"
			}`))
			event.Body = `{
				"messageattributes": {
					"_datadog": {
						"type": "Binary",
						"value": "` + encoded + `"
					}
				}
			}`
		}
		return event
	}

	ctx := func(traceID string) *convertedTraceContext {
		id, err := strconv.ParseUint(traceID, 10, 64)
		assert.Nil(t, err)
		return &convertedTraceContext{
			TraceID:  &id,
			ParentID: &id,
		}
	}

	testcases := []struct {
		name   string
		event  events.SQSMessage
		expect *convertedTraceContext
	}{
		{
			name:   "xray-ddog, _datadog, sns",
			event:  makeEvent(true, false, true, true),
			expect: ctx(_xrayDdogID),
		},
		{
			name:   "xray-ddog, _datadog",
			event:  makeEvent(true, false, true, false),
			expect: ctx(_xrayDdogID),
		},
		{
			name:   "xray-ddog, sns",
			event:  makeEvent(true, false, false, true),
			expect: ctx(_xrayDdogID),
		},
		{
			name:   "xray-ddog",
			event:  makeEvent(true, false, false, false),
			expect: ctx(_xrayDdogID),
		},
		{
			name:   "xray, _datadog, sns",
			event:  makeEvent(false, true, true, true),
			expect: ctx(_datadogID),
		},
		{
			name:   "xray, _datadog",
			event:  makeEvent(false, true, true, false),
			expect: ctx(_datadogID),
		},
		{
			name:   "xray, sns",
			event:  makeEvent(false, true, false, true),
			expect: ctx(_snsID),
		},
		{
			name:   "xray",
			event:  makeEvent(false, true, false, false),
			expect: nil,
		},
		{
			name:   "_datadog, sns",
			event:  makeEvent(false, false, true, true),
			expect: ctx(_datadogID),
		},
		{
			name:   "_datadog",
			event:  makeEvent(false, false, true, false),
			expect: ctx(_datadogID),
		},
		{
			name:   "sns",
			event:  makeEvent(false, false, false, true),
			expect: ctx(_snsID),
		},
		{
			name:   "none",
			event:  makeEvent(false, false, false, false),
			expect: nil,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			actual := extractTraceContext(tc.event)
			assert.Equal(tc.expect, actual)
		})
	}
}
