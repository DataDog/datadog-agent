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

	log.Debug("Creating an inferred span for API Gateway")
	requestContext := attributes.RequestContext
	headers := attributes.Headers
	resource := fmt.Sprintf("%s %s", attributes.HttpMethod, attributes.Path)
	httpurl := fmt.Sprintf("%s%s", requestContext.Domain, attributes.Path)

	// create and add an inferred span to the map of inferredSpans
	var inferredSpan InferredSpan
	inferredSpan.Span = &pb.Span{}
	inferredSpan.Span.SpanID = rand.Random.Uint64()
	inferredSpan.Span.TraceID = rand.Random.Uint64()
	inferredSpan.Span.ParentID = inferredSpan.Span.TraceID
	inferredSpan.Span.Name = "aws.apigateway"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = resource
	inferredSpan.Span.Start = requestContext.RequestTimeEpoch
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

	// Check for synchronicity
	inferredSpan.IsAsync = false
	if headers.InvocationType == "Event" {
		inferredSpan.IsAsync = true
		log.Debug("THIS IS ASYNC")
	}

	// Set the key with the invocation's request ID, not the event payload id
	InferredSpans[ctx.LastRequestID] = inferredSpan
	ctx.IsInferredSpan = true

}
