package inferredspan

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func CreateInferredSpanFromAPIGatewayEvent(eventSource string, attributes EventKeys, inferredSpan InferredSpan) {

	log.Debug("Creating an inferred span for a REST API Gateway")
	requestContext := attributes.RequestContext
	resource := fmt.Sprintf("%s %s", attributes.HttpMethod, attributes.Path)
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, attributes.Path)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)

	inferredSpan.Span.Name = "aws.apigateway"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = resource
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		ApiId:         requestContext.ApiId,
		ApiName:       requestContext.ApiId,
		Endpoint:      attributes.Path,
		HttpUrl:       httpurl,
		OperationName: "aws.apigateway.rest",
		RequestId:     requestContext.RequestId,
		ResourceName:  resource,
		Stage:         requestContext.Stage,
	}

	setSynchronicity(inferredSpan, attributes)
}

func CreateInferredSpanFromAPIGatewayHTTPEvent(eventSource string, attributes EventKeys, inferredSpan InferredSpan) {
	log.Debug("Creating an inferred span for a HTTP API Gateway")
	requestContext := attributes.RequestContext
	http := requestContext.Http
	path := requestContext.RawPath
	resource := fmt.Sprintf("%s %s", http.Method, path)
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, path)
	startTime := calculateStartTime(requestContext.TimeEpoch)

	inferredSpan.Span.Name = "aws.httpapi"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = resource
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Meta = map[string]string{
		Endpoint:      path,
		HttpUrl:       httpurl,
		HttpMethod:    http.Method,
		HttpProtocol:  http.Protocol,
		HttpSourceIP:  http.SourceIP,
		HttpUserAgent: http.UserAgent,
		OperationName: "aws.httpapi",
		RequestId:     requestContext.RequestId,
		ResourceName:  resource,
	}

	setSynchronicity(inferredSpan, attributes)
}

func CreateInferredSpanFromAPIGatewayWebsocketEvent(eventSource string, attributes EventKeys, inferredSpan InferredSpan) {

	requestContext := attributes.RequestContext
	endpoint := requestContext.RouteKey
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, endpoint)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)

	inferredSpan.Span.Name = "aws.apigateway.websocket"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = endpoint
	inferredSpan.Span.Type = "web"
	inferredSpan.Span.Start = startTime
	inferredSpan.Span.Meta = map[string]string{
		ApiId:            requestContext.ApiId,
		ApiName:          requestContext.ApiId,
		ConnectionId:     requestContext.ConnectionID,
		Endpoint:         endpoint,
		EventType:        requestContext.EventType,
		HttpUrl:          httpurl,
		MessageDirection: requestContext.MessageDirection,
		OperationName:    "aws.apigateway.websocket",
		RequestId:        requestContext.RequestId,
		ResourceName:     endpoint,
		Stage:            requestContext.Stage,
	}

	setSynchronicity(inferredSpan, attributes)
}

func setSynchronicity(span InferredSpan, attributes EventKeys) {
	span.IsAsync = false
	if attributes.Headers.InvocationType == "Event" {
		span.IsAsync = true
	}
}

func calculateStartTime(epoch int64) int64 {
	return (epoch / 1000) * 1e9
}
