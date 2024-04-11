package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEndpoint_GetEndpointWithSubdomain(t *testing.T) {
	// Create a test endpoint with a subdomain
	endpoint := Endpoint{
		Subdomain: "https://example",
		Route:     "/api/v1/data",
		Name:      "TestEndpoint",
	}
	result := endpoint.GetEndpoint()
	expected := "https://example/api/v1/data"
	assert.Equal(t, expected, result, "Expected endpoint URL %s, but got %s", expected, result)

	endpoint = Endpoint{
		Subdomain: "https://example/",
		Route:     "/api/v1/data",
		Name:      "TestEndpoint",
	}
	result = endpoint.GetEndpoint()
	assert.Equal(t, expected, result, "Expected endpoint URL %s, but got %s", expected, result)

	endpoint = Endpoint{
		Subdomain: "https://example",
		Route:     "api/v1/data",
		Name:      "TestEndpoint",
	}
	result = endpoint.GetEndpoint()
	assert.Equal(t, expected, result, "Expected endpoint URL %s, but got %s", expected, result)
}

func TestEndpoint_GetEndpointWithoutSubdomain(t *testing.T) {
	// Create a test endpoint without a subdomain
	endpoint := Endpoint{
		Route: "/api/v1/data",
		Name:  "TestEndpoint",
	}
	result := endpoint.GetEndpoint()
	expected := "https://app.datadoghq.com/api/v1/data"
	assert.Equal(t, expected, result, "Expected endpoint URL %s, but got %s", expected, result)
}
