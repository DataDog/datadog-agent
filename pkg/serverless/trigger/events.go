package trigger

import (
	jsonEncoder "encoding/json"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/json"
)

// AWSEventType corresponds to the various event triggers
// we get from AWS
type AWSEventType int

const (
	// APIGatewayEvent describes an event from classic AWS API Gateways
	APIGatewayEvent AWSEventType = iota

	// APIGatewayV2Event describes an event from AWS API Gateways
	APIGatewayV2Event

	// APIGatewayWebsocketEvent describes an event from websocket AWS API Gateways
	APIGatewayWebsocketEvent

	// ALBEvent describes an event from application load balancers
	ALBEvent

	// CloudWatchEvent describes an event from Cloudwatch
	CloudWatchEvent

	// CloudWatchLogsEvent describes an event from Cloudwatch logs
	CloudWatchLogsEvent

	// CloudFrontRequestEvent describes an event from Cloudfront
	CloudFrontRequestEvent

	// DynamoDBStreamEvent describes an event from DynamoDB
	DynamoDBStreamEvent

	// KinesisStreamEvent describes an event from Kinesis
	KinesisStreamEvent

	// S3Event describes an event from S3
	S3Event

	// SNSEvent describes an event from SNS
	SNSEvent

	// SQSEvent describes an event from SQS
	SQSEvent

	// SNSSQSEvent describes an event from an SQS-wrapped SNS topic
	SNSSQSEvent

	// AppSyncResolverEvent describes an event from Appsync
	AppSyncResolverEvent

	// EventBridgeEvent describes an event from EventBridge
	EventBridgeEvent

	// Unknown describes an unknown event type
	Unknown
)

// eventParseFunc defines the signature of AWS event parsing functions
type eventParseFunc func(map[string]interface{}) bool

// GetEventType takes in a payload string and returns an AWSEventType
// that matches the input payload. Returns `Unknown` if a payload could not be
// matched to an event.
func GetEventType(payload map[string]interface{}) (AWSEventType, error) {
	if isAPIGatewayEvent(payload) {
		return APIGatewayEvent, nil
	}

	if isAPIGatewayV2Event(payload) {
		return APIGatewayV2Event, nil
	}

	if isAPIGatewayWebsocketEvent(payload) {
		return APIGatewayWebsocketEvent, nil
	}

	if isALBEvent(payload) {
		return ALBEvent, nil
	}

	if isCloudFrontRequestEvent(payload) {
		return CloudFrontRequestEvent, nil
	}

	if isCloudwatchEvent(payload) {
		return CloudWatchEvent, nil
	}

	if isCloudwatchLogsEvent(payload) {
		return CloudWatchLogsEvent, nil
	}

	if isDynamoDBStreamEvent(payload) {
		return DynamoDBStreamEvent, nil
	}

	if isKinesisStreamEvent(payload) {
		return KinesisStreamEvent, nil
	}

	if isS3Event(payload) {
		return S3Event, nil
	}

	if isSNSEvent(payload) {
		return SNSEvent, nil
	}

	if isSNSSQSEvent(payload) {
		return SNSSQSEvent, nil
	}

	if isSQSEvent(payload) {
		return SQSEvent, nil
	}

	if isAppSyncResolverEvent(payload) {
		return AppSyncResolverEvent, nil
	}

	if isEventBridgeEvent(payload) {
		return EventBridgeEvent, nil
	}

	return Unknown, nil
}

// Unmarshal unmarshals a payload string into a generic interface
func Unmarshal(payload string) (map[string]interface{}, error) {
	jsonPayload := make(map[string]interface{})
	if err := jsonEncoder.Unmarshal([]byte(payload), &jsonPayload); err != nil {
		return nil, err
	}
	return jsonPayload, nil
}

func isAPIGatewayEvent(event map[string]interface{}) bool {
	return json.GetNestedValue(event, "requestContext", "stage") != nil &&
		json.GetNestedValue(event, "httpMethod") != nil &&
		json.GetNestedValue(event, "resource") != nil
}

func isAPIGatewayV2Event(event map[string]interface{}) bool {
	version, ok := json.GetNestedValue(event, "version").(string)
	if !ok {
		return false
	}
	domainName, ok := json.GetNestedValue(event, "requestContext", "domainName").(string)
	if !ok {
		return false
	}
	return version == "2.0" &&
		json.GetNestedValue(event, "rawQueryString") != nil &&
		!strings.Contains(domainName, "lambda-url")
}

func isAPIGatewayWebsocketEvent(event map[string]interface{}) bool {
	return json.GetNestedValue(event, "requestContext") != nil &&
		json.GetNestedValue(event, "messageDirection") != nil
}

func isALBEvent(event map[string]interface{}) bool {
	return json.GetNestedValue(event, "requestContext", "elb") != nil
}

func isCloudwatchEvent(event map[string]interface{}) bool {
	source, ok := json.GetNestedValue(event, "source").(string)
	return ok && source == "aws.events"
}

func isCloudwatchLogsEvent(event map[string]interface{}) bool {
	return json.GetNestedValue(event, "awslogs") != nil
}

func isCloudFrontRequestEvent(event map[string]interface{}) bool {
	return eventRecordsKeyExists(event, "cf")
}

func isDynamoDBStreamEvent(event map[string]interface{}) bool {
	return eventRecordsKeyExists(event, "dynamodb")
}

func isKinesisStreamEvent(event map[string]interface{}) bool {
	return eventRecordsKeyExists(event, "kinesis")
}

func isS3Event(event map[string]interface{}) bool {
	return eventRecordsKeyExists(event, "s3")
}

func isSNSEvent(event map[string]interface{}) bool {
	return eventRecordsKeyExists(event, "Sns")
}

func isSQSEvent(event map[string]interface{}) bool {
	return eventRecordsKeyEquals(event, "eventSource", "aws:sqs")
}

func isSNSSQSEvent(event map[string]interface{}) bool {
	if !eventRecordsKeyEquals(event, "eventSource", "aws:sqs") {
		return false
	}
	messageType, ok := json.GetNestedValue(event, "body", "Type").(string)
	if !ok {
		return false
	}

	topicArn, ok := json.GetNestedValue(event, "body", "TopicArn").(string)
	if !ok {
		return false
	}

	return messageType == "Notification" && topicArn != ""
}

func isAppSyncResolverEvent(event map[string]interface{}) bool {
	return json.GetNestedValue(event, "info", "selectionSetGraphQL") != nil
}

func isEventBridgeEvent(event map[string]interface{}) bool {
	return json.GetNestedValue(event, "detail-type") != nil && json.GetNestedValue(event, "source") != "aws.events"
}

func eventRecordsKeyExists(event map[string]interface{}, key string) bool {
	records, ok := json.GetNestedValue(event, "Records").([]interface{})
	if !ok {
		return false
	}
	if len(records) > 0 {
		if records[0].(map[string]interface{})[key] != nil {
			return true
		}
	}
	return false
}

func eventRecordsKeyEquals(event map[string]interface{}, key string, val string) bool {
	records, ok := json.GetNestedValue(event, "Records").([]interface{})
	if !ok {
		return false
	}
	if len(records) > 0 {
		if mapVal := records[0].(map[string]interface{})[key]; mapVal != nil {
			return mapVal == val
		}
	}
	return false
}
