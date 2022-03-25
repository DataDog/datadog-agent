package inferredspan

import (
	"time"

	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type InferredSpan struct {
	Span      *pb.Span
	IsCreated bool
	TagList   map[string]string
	IsAsync   bool
}

var inferredSpan InferredSpan

func CreateInferredSpanFromAPIGatewayEvent(
	eventSource string,
	ctx *serverlessLog.ExecutionContext,
	attributes EventKeys) {

	log.Debug("CREATING INFERRED SPAN")
	// make sure we reset the inferred span

	requestContext := attributes.RequestContext
	headers := attributes.Headers
	inferredSpan.TagList = map[string]string{
		"operation_name": "aws.apigateway.rest",
		"http.url":       requestContext.Domain + attributes.Path,
		"endpoint":       attributes.Path,
		"resource_names": attributes.HttpMethod + " " + attributes.Path,
		"apiid":          requestContext.ApiId,
		"apiname":        requestContext.ApiId,
		"stage":          requestContext.Stage,
		"request_id":     requestContext.RequestId,
	}

	// Check for synchronicity
	inferredSpan.IsAsync = false
	if headers.InvocationType == "Event" {
		inferredSpan.IsAsync = true
		log.Debug("THIS IS ASYNC")
	}

	inferredSpan.Span = &pb.Span{}
	inferredSpan.Span.Name = "aws.apigateway"
	inferredSpan.Span.Service = requestContext.Domain
	inferredSpan.Span.Resource = attributes.HttpMethod + " " + attributes.Path
	inferredSpan.Span.Start = requestContext.RequestTimeEpoch / 1000
	inferredSpan.Span.Type = "http"
	inferredSpan.Span.Meta = inferredSpan.TagList
	inferredSpan.Span.ParentID = headers.ParentId
	// Mark span created
	inferredSpan.IsCreated = true
}

func CompleteInferredSpan(processTrace func(p *api.Payload), endTime time.Time, isError bool) {
	if inferredSpan.IsCreated {
		duration := endTime.UnixNano() - inferredSpan.Span.Start
		inferredSpan.Span.Duration = duration

		if isError {
			inferredSpan.Span.Error = 1
		}

		traceChunk := &pb.TraceChunk{
			Priority: int32(sampler.PriorityNone),
			Spans:    []*pb.Span{inferredSpan.Span},
		}

		tracerPayload := &pb.TracerPayload{
			Chunks: []*pb.TraceChunk{traceChunk},
		}

		processTrace(&api.Payload{
			Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
			TracerPayload: tracerPayload,
		})

		inferredSpan.IsCreated = false

	} else {
		log.Debug("No inferred spans to submit at this time")
	}
}
