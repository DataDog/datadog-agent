package inferredspan

const (
	OperationName = "operation_name"
	Http          = "http"
	HttpUrl       = "http.url"
	HttpMethod    = "http.method"
	HttpProtocol  = "http.protocol"
	HttpSourceIP  = "http.source_ip"
	HttpUserAgent = "http.user_agent"
	Endpoint      = "endpoint"
	ResourceName  = "resource_names"
	ApiId         = "apiid"
	ApiName       = "apiname"
	Stage         = "stage"
	RequestId     = "request_id"
	API_GATEWAY   = "apigateway"
	HTTP_API      = "http-api"
	WEBSOCKET     = "websocket"
	UNKNOWN       = "unknown"
)

// These keys are used to tell us what event type we received

type EventKeys struct {
	RequestContext RequestContextKeys `json:"requestContext"`
	Headers        HeaderKeys         `json:"headers"`
	HttpMethod     string             `json:"httpMethod"`
	Path           string             `json:"path"`
}

// Request_context is nested in the payload.
// We want to pull out what we need for all event types
type RequestContextKeys struct {
	Stage            string   `json:"stage"`
	RouteKey         string   `json:"routeKey"`
	MessageDirection string   `json:"messageDirection"`
	Domain           string   `json:"domainName"`
	ApiId            string   `json:"apiId"`
	RawPath          string   `json:"rawPath"`
	RequestId        string   `json:"requestID"`
	RequestTimeEpoch int64    `json:"requestTimeEpoch"`
	Http             HttpKeys `json:"http"`
}

type HeaderKeys struct {
	InvocationType string `json:"X-Amz-Invocation-Type"`
	ParentId       uint64 `json:"x-datadog-parent-id"`
}

type HttpKeys struct {
	Method    string `json:"method"`
	Protocol  string `json:"protocol"`
	SourceIP  string `json:"sourceIp"`
	UserAgent string `json:"userAgent"`
}
