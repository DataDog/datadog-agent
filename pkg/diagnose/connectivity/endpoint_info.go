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

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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
	return &gogen.SketchPayload{
		Sketches: []gogen.SketchPayload_Sketch{},
	}
}

func getEndpointsInfo(cfg model.Reader) []endpointInfo {
	emptyPayload := []byte("{}")
	unmarshalledSketchPayload := buildSketchPayload()
	sketchPayload, err := proto.Marshal(unmarshalledSketchPayload)
	if err != nil {
		log.Fatalf("Failed to marshal SketchPayload: %v", err)
	}

	checkRunPayload := []byte("[{\"check\": \"test\", \"status\": 0}]")

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
