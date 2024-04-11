// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package endpoints stores a collection of `transaction.Endpoint` mainly used by the forwarder package to send data to
// Datadog using the right request path for a given type of data.
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
