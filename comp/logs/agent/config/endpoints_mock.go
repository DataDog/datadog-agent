// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"time"

	"go.uber.org/atomic"
)

// NewMockEndpoint creates a new reliable endpoint with default test values
func NewMockEndpoint() Endpoint {
	return Endpoint{
		apiKey:            atomic.NewString("mock-api-key"),
		configSettingPath: "api_key",
		Host:              "localhost",
		Port:              443,
		useSSL:            true,
		isReliable:        true,
		UseCompression:    true,
		CompressionLevel:  6,
	}
}

// NewMockEndpoints creates a new Endpoints struct with a single reliable endpoint
func NewMockEndpoints(endpoints []Endpoint) *Endpoints {
	main := NewMockEndpoint()
	return &Endpoints{
		Main:                   main,
		Endpoints:              endpoints,
		UseHTTP:                true,
		BatchWait:              5 * time.Second,
		BatchMaxSize:           100,
		BatchMaxContentSize:    1000000,
		BatchMaxConcurrentSend: 20,
		InputChanSize:          100,
	}
}

// NewMockEndpointsWithOptions creates a new Endpoints struct with customizable individual endpoints and options
func NewMockEndpointsWithOptions(endpointArray []Endpoint, opts map[string]interface{}) *Endpoints {
	endpoints := NewMockEndpoints(endpointArray)

	if useHTTP, ok := opts["use_http"].(bool); ok {
		endpoints.UseHTTP = useHTTP
	}
	if batchMaxConcurrentSend, ok := opts["batch_max_concurrent_send"].(int); ok {
		endpoints.BatchMaxConcurrentSend = batchMaxConcurrentSend
	}

	return endpoints
}

// NewMockEndpointWithOptions creates a new reliable endpoint with customizable options
func NewMockEndpointWithOptions(opts map[string]interface{}) Endpoint {
	e := NewMockEndpoint()

	if host, ok := opts["host"].(string); ok {
		e.Host = host
	}
	if port, ok := opts["port"].(int); ok {
		e.Port = port
	}
	if apiKey, ok := opts["api_key"].(string); ok {
		e.apiKey = atomic.NewString(apiKey)
	}
	if useSSL, ok := opts["use_ssl"].(bool); ok {
		e.useSSL = useSSL
	}
	if useCompression, ok := opts["use_compression"].(bool); ok {
		e.UseCompression = useCompression
	}
	if compressionLevel, ok := opts["compression_level"].(int); ok {
		e.CompressionLevel = compressionLevel
	}
	if isReliable, ok := opts["is_reliable"].(bool); ok {
		e.isReliable = isReliable
	}

	return e
}
