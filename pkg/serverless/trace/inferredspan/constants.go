// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

const (
	// Below are used for inferred span tagging and enrichment
	apiID            = "apiid"
	apiName          = "apiname"
	connectionID     = "connection_id"
	endpoint         = "endpoint"
	eventType        = "event_type"
	eventSourceArn   = "event_source_arn"
	httpURL          = "http.url"
	httpMethod       = "http.method"
	httpProtocol     = "http.protocol"
	httpSourceIP     = "http.source_ip"
	httpUserAgent    = "http.user_agent"
	messageDirection = "message_direction"
	messageID        = "message_id"
	operationName    = "operation_name"
	queueName        = "queuename"
	receiptHandle    = "receipt_handle"
	requestID        = "request_id"
	resourceNames    = "resource_names"
	senderID         = "sender_id"
	sentTimestamp    = "SentTimestamp"
	stage            = "stage"
	subject          = "subject"
	topicName        = "topicname"
	topicARN         = "topic_arn"
	metadataType     = "type"

	// Below are used for parsing and setting the event sources
	sns = "sns"

	// invocationType is used to look for the invocation type
	// in the payload headers
	invocationType = "X-Amz-Invocation-Type"
)
