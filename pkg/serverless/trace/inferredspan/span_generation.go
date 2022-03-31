package inferredspan

import (
	"fmt"
	"math/rand"

	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var random *rand.Rand

func CreateInferredSpanFromAPIGatewayEvent(
	eventSource string,
	ctx *serverlessLog.ExecutionContext,
	attributes EventKeys,
	inferredSpans InferredSpans) {

	log.Debug("Creating an inferred span for API Gateway")
	requestContext := attributes.RequestContext
	headers := attributes.Headers

	inferredSpan := inferredSpans[requestContext.RequestId]
	inferredSpan.Span = &pb.Span{}

	inferredSpan.Span.TraceID = random.Uint64()
	inferredSpan.Span.ParentID = inferredSpan.Span.TraceID
	inferredSpan.Span.Name = "aws.apigateway"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = attributes.HttpMethod + " " + attributes.Path
	inferredSpan.Span.Start = requestContext.RequestTimeEpoch / 1000
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = map[string]string{
		ApiId:         requestContext.ApiId,
		ApiName:       requestContext.ApiId,
		Endpoint:      attributes.Path,
		HttpUrl:       fmt.Sprintf("%s%s", requestContext.Domain, attributes.Path),
		OperationName: "aws.apigateway.rest",
		RequestId:     requestContext.RequestId,
		ResourceName:  fmt.Sprintf("%s %s", attributes.HttpMethod, attributes.Path),
		Stage:         requestContext.Stage,
	}

	// Check for synchronicity
	inferredSpan.IsAsync = false
	if headers.InvocationType == "Event" {
		inferredSpan.IsAsync = true
		log.Debug("THIS IS ASYNC")
	}
}
