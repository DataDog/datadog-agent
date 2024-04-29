// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

const (
	// Below are used for inferred span tagging and enrichment
	apiID            = "apiid"
	apiName          = "apiname"
	bucketARN        = "bucket_arn"
	bucketName       = "bucketname"
	connectionID     = "connection_id"
	detailType       = "detail_type"
	endpoint         = "endpoint"
	eventID          = "event_id"
	eventName        = "event_name"
	eventSourceArn   = "event_source_arn"
	eventType        = "event_type"
	eventVersion     = "event_version"
	httpURL          = "http.url"
	httpMethod       = "http.method"
	httpProtocol     = "http.protocol"
	httpSourceIP     = "http.source_ip"
	httpUserAgent    = "http.user_agent"
	messageDirection = "message_direction"
	messageID        = "message_id"
	metadataType     = "type"
	objectKey        = "object_key"
	objectSize       = "object_size"
	objectETag       = "object_etag"
	operationName    = "operation_name"
	partitionKey     = "partition_key"
	queueName        = "queuename"
	receiptHandle    = "receipt_handle"
	requestID        = "request_id"
	resourceNames    = "resource_names"
	senderID         = "sender_id"
	sentTimestamp    = "SentTimestamp"
	shardID          = "shardid"
	sizeBytes        = "size_bytes"
	stage            = "stage"
	streamName       = "streamname"
	streamViewType   = "stream_view_type"
	subject          = "subject"
	tableName        = "tablename"
	topicName        = "topicname"
	topicARN         = "topic_arn"

	// Below are used for parsing and setting the event sources
	sns = "sns"

	// invocationType is used to look for the invocation type
	// in the payload headers
	invocationType = "X-Amz-Invocation-Type"
)
