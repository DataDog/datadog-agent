// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivity contains logic for connectivity troubleshooting between the Agent
// and Datadog endpoints. It uses HTTP request to contact different endpoints and displays
// some results depending on endpoints responses, if any, and aims to imitate the Forwarder
// behavior in order to get a more relevant troubleshooting.
package connectivity

import (
	"log"
	"time"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/gogo/protobuf/proto"
)

// endpointInfo is a value object that contains all the information we need to
// contact an endpoint to troubleshoot connectivity issues.
// It can be seen as a very lightweight version of transaction.HTTPTransaction.
// One endpointInfo should be defined for each endpoint we want to troubleshoot.
type endpointInfo struct {
	// Endpoint is the API Endpoint we want to contact.
	Endpoint transaction.Endpoint

	// Method is the HTTP request method we want to send to the endpoint.
	Method string

	// Payload is the HTTP request body we want to send to the endpoint.
	Payload []byte

	// ContentType is media type of the payload.
	ContentType string
}

func buildSketchPayload() *gogen.SketchPayload {
	now := time.Now().Unix()

	sketch := &gogen.SketchPayload_Sketch{
		Metric: "example.metric",
		Host:   "my-hostname",
		Distributions: []gogen.SketchPayload_Sketch_Distribution{
			{
				Ts:    now,
				Cnt:   1,
				Min:   0.1,
				Max:   0.5,
				Avg:   0.3,
				Sum:   0.3,
				V:     []float64{0.3},
				G:     []uint32{1},
				Delta: []uint32{0},
				Buf:   []float64{0.3},
			},
		},
		Tags: []string{"tag1:value1", "tag2:value2"},
		Metadata: &gogen.Metadata{
			Origin: &gogen.Origin{
				OriginProduct:  1234,
				OriginCategory: 5678,
				OriginService:  9012,
			},
		},
	}

	metadata := &gogen.CommonMetadata{
		AgentVersion: "1.0.0",
		Timezone:     "UTC",
		CurrentEpoch: float64(now),
		InternalIp:   "10.0.0.1",
		PublicIp:     "1.2.3.4",
		ApiKey:       "your-api-key",
	}

	payload := &gogen.SketchPayload{
		Sketches: []gogen.SketchPayload_Sketch{*sketch},
		Metadata: *metadata,
	}

	return payload
}

func getEndpointsInfo(cfg model.Reader) []endpointInfo {
	emptyPayload := []byte("{}")
	unmarshalledSketchPayload := buildSketchPayload()
	sketchPayload, err := proto.Marshal(unmarshalledSketchPayload)
	if err != nil {
		log.Fatalf("Failed to marshal SketchPayload: %v", err)
	}

	checkRunPayload := []byte("{\"check\": \"test\", \"status\": 0}")

	jsonCT := "application/json"
	protoCT := "application/x-protobuf"

	// Each added/modified endpointInfo should be tested on all sites.
	return []endpointInfo{
		// v1 endpoints
		{endpoints.V1SeriesEndpoint, "POST", emptyPayload, jsonCT},
		{endpoints.V1CheckRunsEndpoint, "POST", checkRunPayload, jsonCT},
		{endpoints.V1IntakeEndpoint, "POST", emptyPayload, jsonCT},

		// This endpoint behaves differently depending on `site` when using `emptyPayload`. Do not modify `nil` here !
		{endpoints.V1ValidateEndpoint, "GET", nil, jsonCT},
		{endpoints.V1MetadataEndpoint, "POST", emptyPayload, jsonCT},

		// v2 endpoints
		{endpoints.SeriesEndpoint, "POST", emptyPayload, jsonCT},
		{endpoints.SketchSeriesEndpoint, "POST", sketchPayload, protoCT},

		// Flare endpoint
		{transaction.Endpoint{Route: helpers.GetFlareEndpoint(cfg), Name: "flare"}, "HEAD", nil, jsonCT},
	}
}
