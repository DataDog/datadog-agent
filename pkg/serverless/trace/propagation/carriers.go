// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package propagation manages propagation of trace context headers.
package propagation

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	json "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/serverless/trigger/events"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	awsTraceHeader     = "AWSTraceHeader"
	datadogTraceHeader = "_datadog"
	detailHeader       = "detail"

	rootPrefix     = "Root="
	parentPrefix   = "Parent="
	sampledPrefix  = "Sampled="
	rootPadding    = len(rootPrefix + "1-00000000-00000000")
	parentPadding  = len(parentPrefix)
	sampledPadding = len(sampledPrefix)
)

var rootRegex = regexp.MustCompile("Root=1-[0-9a-fA-F]{8}-00000000[0-9a-fA-F]{16}")

var (
	errorAWSTraceHeaderMismatch     = errors.New("AWSTraceHeader does not match expected regex")
	errorAWSTraceHeaderEmpty        = errors.New("AWSTraceHeader does not contain trace ID and parent ID")
	errorStringNotFound             = errors.New("String value not found in _datadog payload")
	errorUnsupportedDataType        = errors.New("Unsupported DataType in _datadog payload")
	errorNoDDContextFound           = errors.New("No Datadog trace context found")
	errorUnsupportedPayloadType     = errors.New("Unsupported type for _datadog payload")
	errorUnsupportedTypeType        = errors.New("Unsupported type in _datadog payload")
	errorUnsupportedValueType       = errors.New("Unsupported value type in _datadog payload")
	errorUnsupportedTypeValue       = errors.New("Unsupported Type in _datadog payload")
	errorCouldNotUnmarshal          = errors.New("Could not unmarshal the invocation event payload")
	errorNoStepFunctionContextFound = errors.New("no Step Function context found in Step Function event")
)

// extractTraceContextfromAWSTraceHeader extracts trace context from the
// AWSTraceHeader directly. Unlike the other carriers in this file, it should
// not be passed to the tracer.Propagator, instead extracting context directly.
func extractTraceContextfromAWSTraceHeader(value string) (*TraceContext, error) {
	if !rootRegex.MatchString(value) {
		return nil, errorAWSTraceHeaderMismatch
	}
	var (
		startPart int
		traceID   string
		parentID  string
		sampled   string
		err       error
	)
	length := len(value)
	for startPart < length {
		endPart := strings.IndexRune(value[startPart:], ';') + startPart
		if endPart < startPart {
			endPart = length
		}
		part := value[startPart:endPart]
		if strings.HasPrefix(part, rootPrefix) {
			if traceID == "" {
				traceID = part[rootPadding:]
			}
		} else if strings.HasPrefix(part, parentPrefix) {
			if parentID == "" {
				parentID = part[parentPadding:]
			}
		} else if strings.HasPrefix(part, sampledPrefix) {
			if sampled == "" {
				sampled = part[sampledPadding:]
			}
		}
		if traceID != "" && parentID != "" && sampled != "" {
			break
		}
		startPart = endPart + 1
	}
	tc := new(TraceContext)
	tc.TraceID, err = strconv.ParseUint(traceID, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse trace ID from AWSTraceHeader: %w", err)
	}
	tc.ParentID, err = strconv.ParseUint(parentID, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse parent ID from AWSTraceHeader: %w", err)
	}
	if sampled == "1" {
		tc.SamplingPriority = sampler.PriorityAutoKeep
	}
	if tc.TraceID == 0 || tc.ParentID == 0 {
		return nil, errorAWSTraceHeaderEmpty
	}
	return tc, nil
}

// sqsMessageCarrier returns the tracer.TextMapReader used to extract trace
// context from the events.SQSMessage type.
func sqsMessageCarrier(event events.SQSMessage) (tracer.TextMapReader, error) {
	if attr, ok := event.MessageAttributes[datadogTraceHeader]; ok {
		return sqsMessageAttrCarrier(attr)
	}
	return snsSqsMessageCarrier(event)
}

// sqsMessageAttrCarrier returns the tracer.TextMapReader used to extract trace
// context from the events.SQSMessageAttribute field on an events.SQSMessage
// type.
func sqsMessageAttrCarrier(attr events.SQSMessageAttribute) (tracer.TextMapReader, error) {
	var bytes []byte
	switch attr.DataType {
	case "String":
		if attr.StringValue == nil {
			return nil, errorStringNotFound
		}
		bytes = []byte(*attr.StringValue)
	case "Binary":
		// SNS => SQS => Lambda with SQS's subscription to SNS has enabled RAW
		// MESSAGE DELIVERY option
		bytes = attr.BinaryValue // No need to decode base64 because already decoded
	default:
		return nil, errorUnsupportedDataType
	}

	var carrier tracer.TextMapCarrier
	if err := json.Unmarshal(bytes, &carrier); err != nil {
		return nil, fmt.Errorf("Error unmarshaling payload value: %w", err)
	}
	return carrier, nil
}

// snsBody is used to  unmarshal only required fields on events.SNSEntity
// types.
type snsBody struct {
	MessageAttributes map[string]interface{}
}

// snsSqsMessageCarrier returns the tracer.TextMapReader used to extract trace
// context from the body of an events.SQSMessage type.
func snsSqsMessageCarrier(event events.SQSMessage) (tracer.TextMapReader, error) {
	var body snsBody
	err := json.Unmarshal([]byte(event.Body), &body)
	if err != nil {
		return nil, fmt.Errorf("Error unmarshaling message body: %w", err)
	}
	return snsEntityCarrier(events.SNSEntity{
		MessageAttributes: body.MessageAttributes,
	})
}

// snsEntityCarrier returns the tracer.TextMapReader used to extract trace
// context from the attributes of an events.SNSEntity type.
func snsEntityCarrier(event events.SNSEntity) (tracer.TextMapReader, error) {
	// Check if this is a regular SNS message with Datadog trace information
	if msgAttrs, ok := event.MessageAttributes[datadogTraceHeader]; ok {
		return handleRegularSNSMessage(msgAttrs)
	}

	// If not, check if this is an EventBridge event sent through SNS
	var eventBridgeEvent map[string]interface{}
	if err := json.Unmarshal([]byte(event.Message), &eventBridgeEvent); err == nil {
		return handleEventBridgeThroughSNS(eventBridgeEvent)
	}

	return nil, errorNoDDContextFound
}

// handleRegularSNSMessage builds the payload carrier for a regular SNS message.
func handleRegularSNSMessage(msgAttrs interface{}) (tracer.TextMapReader, error) {
	mapAttrs, ok := msgAttrs.(map[string]interface{})
	if !ok {
		return nil, errorUnsupportedPayloadType
	}

	typ, ok := mapAttrs["Type"].(string)
	if !ok {
		return nil, errorUnsupportedTypeType
	}
	val, ok := mapAttrs["Value"].(string)
	if !ok {
		return nil, errorUnsupportedValueType
	}

	var bytes []byte
	var err error
	switch typ {
	case "Binary":
		bytes, err = base64.StdEncoding.DecodeString(val)
		if err != nil {
			return nil, fmt.Errorf("Error decoding binary: %w", err)
		}
	case "String":
		bytes = []byte(val)
	default:
		return nil, errorUnsupportedTypeValue
	}

	var carrier tracer.TextMapCarrier
	if err = json.Unmarshal(bytes, &carrier); err != nil {
		return nil, fmt.Errorf("Error unmarshaling the decoded binary: %w", err)
	}
	return carrier, nil
}

// handleEventBridgeThroughSNS builds payload carrier for an EventBridge
// event wrapped in an SNS message.
func handleEventBridgeThroughSNS(eventBridgeEvent map[string]interface{}) (tracer.TextMapReader, error) {
	detail, ok := eventBridgeEvent[detailHeader].(map[string]interface{})
	if !ok {
		return nil, errorUnsupportedPayloadType
	}

	traceContextAny, ok := detail[datadogTraceHeader].(map[string]interface{})
	if !ok {
		return nil, errorNoDDContextFound
	}

	// Some values are integers, not strings, so we have to convert before using tracer.TextMapCarrier()
	traceContext := make(map[string]string)
	for key, val := range traceContextAny {
		traceContext[key] = fmt.Sprintf("%v", val)
	}

	carrier := tracer.TextMapCarrier(traceContext)
	return carrier, nil
}

// eventBridgeCarrier returns the tracer.TextMapReader used to extract trace
// context from the Detail field of an events.EventBridgeEvent
func eventBridgeCarrier(event events.EventBridgeEvent) (tracer.TextMapReader, error) {
	traceContext := event.Detail.TraceContext
	if len(traceContext) > 0 {
		return tracer.TextMapCarrier(traceContext), nil
	}
	return nil, errorNoDDContextFound
}

type invocationPayload struct {
	Headers tracer.TextMapCarrier `json:"headers"`
}

// rawPayloadCarrier returns the tracer.TextMapReader used to extract trace
// context from the raw json event payload.
func rawPayloadCarrier(rawPayload []byte) (tracer.TextMapReader, error) {
	var payload invocationPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return nil, errorCouldNotUnmarshal
	}
	return payload.Headers, nil
}

// headersCarrier returns the tracer.TextMapReader used to extract trace
// context from a Headers field of form map[string]string.
func headersCarrier(hdrs map[string]string) (tracer.TextMapReader, error) {
	return tracer.TextMapCarrier(hdrs), nil
}

// extractTraceContextFromStepFunctionContext extracts the execution ARN, state name, and state entered time and uses them to generate Trace ID and Parent ID
// The logic is based on the trace context conversion in Logs To Traces, dd-trace-py, dd-trace-js, etc.
func extractTraceContextFromStepFunctionContext(event events.StepFunctionPayload) (*TraceContext, error) {
	tc := new(TraceContext)

	execArn := event.Execution.ID
	stateName := event.State.Name
	stateEnteredTime := event.State.EnteredTime

	if execArn == "" || stateName == "" || stateEnteredTime == "" {
		return nil, errorNoStepFunctionContextFound
	}

	lowerTraceID, upperTraceID := stringToDdTraceIDs(execArn)
	parentID := stringToDdSpanID(execArn, stateName, stateEnteredTime)

	tc.TraceID = lowerTraceID
	tc.TraceIDUpper64Hex = upperTraceID
	tc.ParentID = parentID
	tc.SamplingPriority = sampler.PriorityAutoKeep
	return tc, nil
}

// stringToDdSpanID hashes the Execution ARN, state name, and state entered time to generate a 64-bit span ID
func stringToDdSpanID(execArn string, stateName string, stateEnteredTime string) uint64 {
	uniqueSpanString := fmt.Sprintf("%s#%s#%s", execArn, stateName, stateEnteredTime)
	spanHash := sha256.Sum256([]byte(uniqueSpanString))
	parentID := getPositiveUInt64(spanHash[0:8])
	return parentID
}

// stringToDdTraceIDs hashes an Execution ARN to generate the lower and upper 64 bits of a 128-bit trace ID
func stringToDdTraceIDs(toHash string) (uint64, string) {
	hash := sha256.Sum256([]byte(toHash))
	lower64 := getPositiveUInt64(hash[8:16])
	upper64 := getHexEncodedString(getPositiveUInt64(hash[0:8]))
	return lower64, upper64
}

// getPositiveUInt64 converts the first 8 bytes of a byte array to a positive uint64
func getPositiveUInt64(hashBytes []byte) uint64 {
	var result uint64
	for i := 0; i < 8; i++ {
		result = (result << 8) + uint64(hashBytes[i])
	}
	result &= ^uint64(1 << 63) // Ensure the highest bit is always 0
	if result == 0 {
		return 1
	}
	return result
}

func getHexEncodedString(toEncode uint64) string {
	//return hex.EncodeToString(hashBytes[:8])
	return fmt.Sprintf("%x", toEncode) //maybe?
}
