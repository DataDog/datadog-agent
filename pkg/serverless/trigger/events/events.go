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
