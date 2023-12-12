package events

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
