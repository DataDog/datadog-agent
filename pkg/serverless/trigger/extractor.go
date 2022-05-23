package trigger

// ExtractionFunction is the signature of a function which takes in a payload
// event and returns the ARN or identifier of the event type.
type ExtractionFunction func(map[string]interface{}) string

// ExtractEventARN takes in an event and returns the source of the payload,
// such as SNS, SQS, etc..
func ExtractEventARN(event map[string]interface{}) (string, error) {
	eventExtractors := map[AWSEventType]ExtractionFunction{
		ApiGatewayEvent:          extractApiGatewayEventARN,
		ApiGatewayV2Event:        extractApiGatewayV2EventARN,
		ApiGatewayWebsocketEvent: extractApiGatewayWebSocketEventARN,
		AlbEvent:                 extractAlbEventARN,
		CloudWatchEvent:          extractCloudwatchEventARN,
		CloudWatchLogsEvent:      extractCloudwatchLogsEventARN,
		CloudFrontRequestEvent:   extractCloudFrontRequestEventARN,
		DynamoDBStreamEvent:      extractDynamoDBStreamEventARN,
		KinesisStreamEvent:       extractKinesisStreamEventARN,
		S3Event:                  extractS3EventArn,
		SNSEvent:                 extractSNSEventArn,
		SQSEvent:                 extractSQSEventARN,
		SNSSQSEvent:              extractSNSSQSEventARN,
		AppSyncResolverEvent:     extractAppSyncResolverEventARN,
		EventBridgeEvent:         extractEventBridgeEventARN,
	}

	eventType, err := GetEventType(event)
	if err != nil {
		return "", err
	}

	return eventExtractors[eventType](event), nil
}

func extractApiGatewayEventARN(event map[string]interface{}) string {
	return ""
}

func extractApiGatewayV2EventARN(event map[string]interface{}) string {
	return ""
}

func extractApiGatewayWebSocketEventARN(event map[string]interface{}) string {
	return ""
}

func extractAlbEventARN(event map[string]interface{}) string {
	return ""
}

func extractCloudwatchEventARN(event map[string]interface{}) string {
	return ""
}

func extractCloudwatchLogsEventARN(event map[string]interface{}) string {
	return ""
}

func extractCloudFrontRequestEventARN(event map[string]interface{}) string {
	return ""
}

func extractDynamoDBStreamEventARN(event map[string]interface{}) string {
	return ""
}

func extractKinesisStreamEventARN(event map[string]interface{}) string {
	return ""
}

func extractS3EventArn(event map[string]interface{}) string {
	return ""
}

func extractSNSEventArn(event map[string]interface{}) string {
	return ""
}

func extractSQSEventARN(event map[string]interface{}) string {
	return ""
}

func extractSNSSQSEventARN(event map[string]interface{}) string {
	return ""
}

func extractAppSyncResolverEventARN(event map[string]interface{}) string {
	return ""
}

func extractEventBridgeEventARN(event map[string]interface{}) string {
	return ""
}
