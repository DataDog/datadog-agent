package inferredspan

import (
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
)

func CreateInferredSpanFromAPIGatewayEvent(eventSource string, ctx serverlessLog.ExecutionContext) {

}

// def create_inferred_span_from_api_gateway_event(event, context):
//     request_context = event["requestContext"]
//     domain = request_context["domainName"]
//     method = event["httpMethod"]
//     path = event["path"]
//     resource = "{0} {1}".format(method, path)
//     tags = {
//         "operation_name": "aws.apigateway.rest",
//         "http.url": domain + path,
//         "endpoint": path,
//         "http.method": method,
//         "resource_names": resource,
//         "apiid": request_context["apiId"],
//         "apiname": request_context["apiId"],
//         "stage": request_context["stage"],
//         "request_id": request_context["requestId"],
//     }
//     request_time_epoch = request_context["requestTimeEpoch"]
//     if is_api_gateway_invocation_async(event):
//         InferredSpanInfo.set_tags(tags, tag_source="self", synchronicity="async")
//     else:
//         InferredSpanInfo.set_tags(tags, tag_source="self", synchronicity="sync")
//     args = {
//         "service": domain,
//         "resource": resource,
//         "span_type": "http",
//     }
//     tracer.set_tags({"_dd.origin": "lambda"})
//     span = tracer.trace("aws.apigateway", **args)
//     if span:
//         span.set_tags(tags)
//     span.start = request_time_epoch / 1000
//     return span
