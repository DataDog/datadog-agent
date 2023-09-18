// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// PayloadParsedStore contains parsers and a store of parsed payloads
type PayloadParsedStore struct {
	payloadJSONStore map[string][]api.ParsedPayload
	parserMap        map[string]func(api.Payload) (interface{}, error)
}

// NewPayloadParsedStore creates a new empty Datadog payload parser
func NewPayloadParsedStore() PayloadParsedStore {
	parser := PayloadParsedStore{
		payloadJSONStore: map[string][]api.ParsedPayload{},
		parserMap:        map[string]func(api.Payload) (interface{}, error){},
	}
	parser.parserMap["/api/v2/logs"] = parser.getLogPayLoadJSON
	parser.parserMap["/api/v2/series"] = parser.getMetricPayLoadJSON
	parser.parserMap["/api/v1/check_run"] = parser.getCheckRunPayLoadJSON
	parser.parserMap["/api/v1/connections"] = parser.getConnectionsPayLoadProtobuf
	parser.payloadJSONStore["/api/v2/logs"] = []api.ParsedPayload{}
	parser.payloadJSONStore["/api/v2/series"] = []api.ParsedPayload{}
	parser.payloadJSONStore["/api/v1/check_run"] = []api.ParsedPayload{}
	parser.payloadJSONStore["/api/v1/connections"] = []api.ParsedPayload{}
	return parser
}

func (fi *PayloadParsedStore) getJSONPayload(route string) ([]api.ParsedPayload, error) {
	payload, ok := fi.payloadJSONStore[route]
	if ok {
		return payload, nil
	}
	return nil, fmt.Errorf("route %s isn't supported", route)
}

// IsRouteHandled checks if a route is handled by the Datadog parsed store
func (fi *PayloadParsedStore) IsRouteHandled(route string) bool {
	_, ok := fi.parserMap[route]
	return ok
}

func (fi *PayloadParsedStore) getLogPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseLogPayload(payload)
}

func (fi *PayloadParsedStore) getMetricPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseMetricSeries(payload)
}

func (fi *PayloadParsedStore) getCheckRunPayLoadJSON(payload api.Payload) (interface{}, error) {
	return aggregator.ParseCheckRunPayload(payload)
}

func (fi *PayloadParsedStore) getConnectionsPayLoadProtobuf(payload api.Payload) (interface{}, error) {
	return aggregator.ParseConnections(payload)
}

func (fi *PayloadParsedStore) parseAndAppend(payload api.Payload, route string) error {
	parsedPayload := api.ParsedPayload{
		Timestamp: payload.Timestamp,
		Data:      "",
		Encoding:  payload.Encoding,
	}

	if payloadFunc, ok := fi.parserMap[route]; ok {
		var err error
		parsedPayload.Data, err = payloadFunc(payload)
		if err != nil {
			return err
		}

		fi.payloadJSONStore[route] = append(fi.payloadJSONStore[route], parsedPayload)
	}
	return nil
}

// Clean delete any stored data
func (fi *PayloadParsedStore) Clean() {
	for k := range fi.payloadJSONStore {
		delete(fi.payloadJSONStore, k)
	}
}
