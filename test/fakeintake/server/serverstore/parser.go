// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverstore

import (
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type parserFunc func(api.Payload) (interface{}, error)

var parserMap = map[string]parserFunc{
	"/api/v2/logs":        getLogPayLoadJSON,
	"/api/v2/series":      getMetricPayLoadJSON,
	"/api/v1/check_run":   getCheckRunPayLoadJSON,
	"/api/v1/connections": getConnectionsPayLoadProtobuf,
}

func getLogPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseLogPayload(payload)
}

func getMetricPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseMetricSeries(payload)
}

func getCheckRunPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseCheckRunPayload(payload)
}

func getConnectionsPayLoadProtobuf(payload api.Payload) (interface{}, error) {
	return aggregator.ParseConnections(payload)
}

// IsRouteHandled checks if a route is handled by the Datadog parsed store
func IsRouteHandled(route string) bool {
	_, ok := parserMap[route]
	return ok
}
