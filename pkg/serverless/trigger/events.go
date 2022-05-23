package trigger

import (
	jsonEncoder "encoding/json"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/json"
)

type AWSEventType int

const (
	ApiGatewayEvent AWSEventType = iota
	ApiGatewayV2Event
	ApiGatewayWebsocketEvent
	AlbEvent
	CloudWatchEvent
	CloudWatchLogsEvent
	CloudFrontRequestEvent
	DynamoDBStreamEvent
	KinesisStreamEvent
	S3Event
	SNSEvent
	SQSEvent
	SNSSQSEvent
	AppSyncResolverEvent
	EventBridgeEvent
	Unknown
)

// EventParseFunc defines the signature of AWS event parsing functions
type EventParseFunc func(map[string]interface{}) bool

// GetEventType takes in a payload string and returns an AWSEventType
// that matches the input payload. Returns `Unknown` if a payload could not be
// matched to an event.
func GetEventType(payload map[string]interface{}) (AWSEventType, error) {
	parseFuncs := map[AWSEventType]EventParseFunc{
		ApiGatewayEvent:          isApiGatewayEvent,
		ApiGatewayV2Event:        isApiGatewayV2Event,
		ApiGatewayWebsocketEvent: isApiGatewayWebsocketEvent,
		AlbEvent:                 isALBEvent,
		CloudFrontRequestEvent:   isCloudFrontRequestEvent,
		CloudWatchEvent:          isCloudwatchEvent,
		CloudWatchLogsEvent:      isCloudwatchLogsEvent,
		DynamoDBStreamEvent:      isDynamoDBStreamEvent,
		KinesisStreamEvent:       isKinesisStreamEvent,
		S3Event:                  isS3Event,
		SNSEvent:                 isSNSEvent,
		SNSSQSEvent:              isSNSSQSEvent,
		SQSEvent:                 isSQSEvent,
		AppSyncResolverEvent:     isAppSyncResolverEvent,
		EventBridgeEvent:         isEventBridgeEvent,
	}

	for enum, parseFunc := range parseFuncs {
		if parseFunc(payload) {
			return enum, nil
		}
	}

	return Unknown, nil
}

func Unmarshal(payload string) (map[string]interface{}, error) {
	jsonPayload := make(map[string]interface{})
	if err := jsonEncoder.Unmarshal([]byte(payload), &jsonPayload); err != nil {
		return nil, err
	}
	return jsonPayload, nil
}

func isApiGatewayEvent(event map[string]interface{}) bool {
	return json.GetNestedValue(event, "requestContext", "stage") != nil &&
		json.GetNestedValue(event, "httpMethod") != nil &&
		json.GetNestedValue(event, "resource") != nil
}

func isApiGatewayV2Event(event map[string]interface{}) bool {
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

func isApiGatewayWebsocketEvent(event map[string]interface{}) bool {
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
	return json.GetNestedValue(event, "detail-type") != nil
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
