package inferredspan

import (
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
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

	rawTraceContext := extractTraceContextFromPureSqsEvent(mockSQSMessage)
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

	rawTraceContext := extractTraceContextFromPureSqsEvent(mockSQSMessageAttribute)

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

	rawTraceContext := extractTraceContextFromPureSqsEvent(mockSQSMessageAttribute)

	assert.Nil(t, rawTraceContext)
}

func TestExtractContextFromPureSqsEvent_InvalidJson(t *testing.T) {
	mockSQSMessageAttribute := events.SQSMessageAttribute{
		DataType:    "String",
		StringValue: aws.String(`invalid json`),
	}

	rawTraceContext := extractTraceContextFromPureSqsEvent(mockSQSMessageAttribute)
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

	rawTraceContext := extractTraceContextFromPureSqsEvent(mockSQSMessageAttribute)
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
