package inferredspan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseEventSourceUnknown(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"httpMethod":"GET"}}`
	str, _ := ParseEventSource(testString)
	assert.Equal(t, str, UNKNOWN)
}
func TestParseEventSourceREST(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"httpMethod":"GET", "stage":"dev"}}`
	str, _ := ParseEventSource(testString)
	assert.Equal(t, str, API_GATEWAY)
}
func TestParseEventSourceHTTP(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"routeKey":"GET /httpapi/get", "stage":"dev"}}`
	str, _ := ParseEventSource(testString)
	assert.Equal(t, str, HTTP_API)
}
func TestParseEventSourceWebsocket(t *testing.T) {
	testString := `{"resource":"/users/create", "httpMethod":"GET","requestContext":{"messageDirection":"IN", "stage":"dev"}}`
	str, _ := ParseEventSource(testString)
	assert.Equal(t, str, WEBSOCKET)
}
