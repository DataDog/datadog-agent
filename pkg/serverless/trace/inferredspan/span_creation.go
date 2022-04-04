package inferredspan

import (
	"fmt"

	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	rand "github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func CreateInferredSpanFromAPIGatewayEvent(
	eventSource string,
	ctx *serverlessLog.ExecutionContext,
	attributes EventKeys) {

	log.Debug("Creating an inferred span for a REST API Gateway")
	requestContext := attributes.RequestContext
	resource := fmt.Sprintf("%s %s", attributes.HttpMethod, attributes.Path)
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, attributes.Path)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)

	inferredSpan := generateSpan()
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
	// Set the key with the invocation's request ID, not the event payload id
	InferredSpans[ctx.LastRequestID] = inferredSpan
	ctx.IsInferredSpan = true
}

func CreateInferredSpanFromAPIGatewayHTTPEvent(
	eventSource string,
	ctx *serverlessLog.ExecutionContext,
	attributes EventKeys) {

	log.Debug("Creating an inferred span for a HTTP API Gateway")
	requestContext := attributes.RequestContext
	http := requestContext.Http
	path := requestContext.RawPath
	resource := fmt.Sprintf("%s %s", http.Method, path)
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, path)
	startTime := calculateStartTime(requestContext.RequestTimeEpoch)

	inferredSpan := generateSpan()
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
	// Set the key with the invocation's request ID, not the event payload id
	InferredSpans[ctx.LastRequestID] = inferredSpan
	ctx.IsInferredSpan = true
}

// returns an inferred span, span id and trace id
func generateSpan() InferredSpan {
	var inferredSpan InferredSpan
	inferredSpan.Span = &pb.Span{}
	inferredSpan.Span.SpanID = rand.Random.Uint64()
	inferredSpan.Span.TraceID = rand.Random.Uint64()
	return inferredSpan
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
