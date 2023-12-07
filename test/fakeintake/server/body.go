// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"net/http"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
)

type errorResponseBody struct {
	Errors []string `json:"errors"`
}

// Same struct as in datadog-agent/comp/core/flare/helpers/send_flare.go
type flareResponseBody struct {
	CaseID int    `json:"case_id,omitempty"`
	Error  string `json:"error,omitempty"`
}

// defaultResponse is the default response returned by the fakeintake server
var defaultResponse httpResponse

func getConnectionsResponse() []byte {
	clStatus := &agentmodel.CollectorStatus{
		ActiveClients: 1,
		Interval:      30,
	}
	response := &agentmodel.ResCollector{
		Message: "",
		Status:  clStatus,
	}
	out, err := agentmodel.EncodeMessage(agentmodel.Message{
		Header: agentmodel.MessageHeader{
			Version:        agentmodel.MessageV3,
			Encoding:       agentmodel.MessageEncodingProtobuf,
			Type:           agentmodel.TypeResCollector,
			OrgID:          503,
			SubscriptionID: 2,
		}, Body: response})
	if err != nil {
		panic(err) // will happen when new Message version exist and mark encodeV3 as retired
	}
	return out
}

// newResponseOverrides creates and returns a map of URL paths to HTTP responses populated with
// static custom response overrides
func newResponseOverrides() map[string]httpResponse {
	return map[string]httpResponse{
		"/support/flare": updateResponseFromData(httpResponse{
			statusCode:  http.StatusOK,
			contentType: "application/json",
			data:        flareResponseBody{CaseID: 0, Error: ""},
		}),
		"/api/v1/connections": updateResponseFromData(httpResponse{
			statusCode:  http.StatusOK,
			contentType: "application/x-protobuf",
			data:        getConnectionsResponse(),
		}),
	}
}
