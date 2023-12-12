package events

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

type APIGatewayProxyRequest struct {
	Resource       string
	Path           string
	HTTPMethod     string
	Headers        map[string]string
	RequestContext APIGatewayProxyRequestContext
}

type APIGatewayProxyRequestContext struct {
	Stage            string
	DomainName       string
	RequestID        string
	Path             string
	HTTPMethod       string
	RequestTimeEpoch int64
	APIID            string
}

type APIGatewayV2HTTPRequest struct {
	RouteKey       string
	Headers        map[string]string
	RequestContext APIGatewayV2HTTPRequestContext
}

type APIGatewayV2HTTPRequestContext struct {
	Stage      string
	RequestID  string
	APIID      string
	DomainName string
	TimeEpoch  int64
	HTTP       APIGatewayV2HTTPRequestContextHTTPDescription
}

type APIGatewayV2HTTPRequestContextHTTPDescription struct {
	Method    string
	Path      string
	Protocol  string
	SourceIP  string
	UserAgent string
}

type APIGatewayWebsocketProxyRequest struct {
	Headers        map[string]string
	RequestContext APIGatewayWebsocketProxyRequestContext
}

type APIGatewayWebsocketProxyRequestContext struct {
	Stage            string
	RequestID        string
	APIID            string
	ConnectionID     string
	DomainName       string
	EventType        string
	MessageDirection string
	RequestTimeEpoch int64
	RouteKey         string
}

type APIGatewayCustomAuthorizerRequest struct {
	Type               string
	AuthorizationToken string
	MethodArn          string
}

type APIGatewayCustomAuthorizerRequestTypeRequest struct {
	MethodArn      string
	Resource       string
	HTTPMethod     string
	Headers        map[string]string
	RequestContext APIGatewayCustomAuthorizerRequestTypeRequestContext
}

type APIGatewayCustomAuthorizerRequestTypeRequestContext struct {
	Path string
}

type ALBTargetGroupRequest struct {
	HTTPMethod     string
	Path           string
	Headers        map[string]string
	RequestContext ALBTargetGroupRequestContext
}

type ALBTargetGroupRequestContext struct {
	ELB ELBContext
}

type ELBContext struct {
	TargetGroupArn string
}

type CloudWatchEvent struct {
	Resources []string
}

type CloudwatchLogsEvent struct {
	AWSLogs CloudwatchLogsRawData
}

type CloudwatchLogsRawData struct {
	Data string
}

func (c CloudwatchLogsRawData) Parse() (d CloudwatchLogsData, err error) {
	data, err := base64.StdEncoding.DecodeString(c.Data)
	if err != nil {
		return
	}

	zr, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return
	}
	defer zr.Close()

	dec := json.NewDecoder(zr)
	err = dec.Decode(&d)

	return
}

type CloudwatchLogsData struct {
	LogGroup string
}

type DynamoDBEvent struct {
	Records []DynamoDBEventRecord
}

type DynamoDBEventRecord struct {
	Change         DynamoDBStreamRecord `json:"dynamodb"`
	EventID        string
	EventName      string
	EventVersion   string
	EventSourceArn string
}

type DynamoDBStreamRecord struct {
	ApproximateCreationDateTime events.SecondsEpochTime
	SizeBytes                   int64
	StreamViewType              string
}

type KinesisEvent struct {
	Records []KinesisEventRecord
}

type KinesisEventRecord struct {
	EventID        string
	EventName      string
	EventSourceArn string
	EventVersion   string
	Kinesis        KinesisRecord
}

type KinesisRecord struct {
	ApproximateArrivalTimestamp events.SecondsEpochTime
	PartitionKey                string
}

type EventBridgeEvent struct {
	DetailType string `json:"detail-type"`
	Source     string
	StartTime  string
}

type S3Event struct {
	Records []S3EventRecord `json:"Records"`
}

type S3EventRecord struct {
	EventSource string
	EventTime   time.Time
	EventName   string
	S3          S3Entity
}

type S3Entity struct {
	Bucket S3Bucket
	Object S3Object
}

type S3Bucket struct {
	Name string
	Arn  string
}

type S3Object struct {
	Key  string
	Size int64
	ETag string
}

type SNSEvent struct {
	Records []SNSEventRecord `json:"Records"`
}

type SNSEventRecord struct {
	SNS SNSEntity
}

type SNSEntity struct {
	MessageID string
	Type      string
	TopicArn  string
	Timestamp time.Time
	Subject   string
}

type SQSEvent struct {
	Records []SQSMessage
}

type SQSMessage struct {
	ReceiptHandle     string
	Body              string
	Attributes        map[string]string
	MessageAttributes map[string]SQSMessageAttribute
	EventSourceARN    string
}

type SQSMessageAttribute struct {
	StringValue *string
	BinaryValue []byte
	DataType    string
}

type LambdaFunctionURLRequest struct {
	Headers        map[string]string
	RequestContext LambdaFunctionURLRequestContext
}

type LambdaFunctionURLRequestContext struct {
	RequestID  string
	APIID      string
	DomainName string
	TimeEpoch  int64
	HTTP       LambdaFunctionURLRequestContextHTTPDescription
}

type LambdaFunctionURLRequestContextHTTPDescription struct {
	Method    string
	Path      string
	Protocol  string
	SourceIP  string
	UserAgent string
}
