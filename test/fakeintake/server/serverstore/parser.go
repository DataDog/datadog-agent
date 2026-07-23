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
	"/api/v2/agentdiscovery":        getAgentDiscoveryPayloadProtobuf,
	"/api/v2/logs":                  getLogPayLoadJSON,
	"/api/v2/series":                getMetricPayLoadJSON,
	"/api/v1/series":                getV1MetricPayLoadJSON,
	"/api/v1/check_run":             getCheckRunPayLoadJSON,
	"/api/v1/connections":           getConnectionsPayLoadProtobuf,
	"/api/beta/sketches":            getSketchPayloadProtobuf,
	"/api/intake/metrics/v3/series": getMetricV3SeriesPayload,
	"/api/v2/apmtelemetry":          getAgentTelemetryLogsJSON,
}

func getAgentDiscoveryPayloadProtobuf(payload api.Payload) (interface{}, error) {
	return aggregator.ParseAgentDiscoveryPayload(payload)
}

func getLogPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseLogPayload(payload)
}

func getMetricPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseMetricSeries(payload)
}

func getV1MetricPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseV1MetricSeries(payload)
}

func getCheckRunPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseCheckRunPayload(payload)
}

func getConnectionsPayLoadProtobuf(payload api.Payload) (interface{}, error) {
	return aggregator.ParseConnections(payload)
}

func getSketchPayloadProtobuf(payload api.Payload) (interface{}, error) {
	return aggregator.ParseSketches(payload)
}

func getMetricV3SeriesPayload(payload api.Payload) (interface{}, error) {
	return aggregator.ParseMetricSeriesV3(payload)
}

func getAgentTelemetryLogsJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseAgentTelemetryLogs(payload)
}

// IsRouteHandled checks if a route is handled by the Datadog parsed store
func IsRouteHandled(route string) bool {
	_, ok := parserMap[route]
	return ok
}
