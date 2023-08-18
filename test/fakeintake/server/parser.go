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

type PayloadParser struct {
	payloadJsonStore map[string][]api.ParsedPayload
	parserMap        map[string]func(api.Payload) (interface{}, error)
}

func NewPayloadParser() PayloadParser {
	parser := PayloadParser{
		payloadJsonStore: map[string][]api.ParsedPayload{},
		parserMap:        map[string]func(api.Payload) (interface{}, error){},
	}
	parser.parserMap["/api/v2/logs"] = parser.getLogPayLoadJson
	parser.parserMap["/api/v2/series"] = parser.getMetricPayLoadJson
	parser.parserMap["/api/v1/check_run"] = parser.getCheckRunPayLoadJson
	parser.payloadJsonStore["/api/v2/logs"] = []api.ParsedPayload{}
	parser.payloadJsonStore["/api/v2/series"] = []api.ParsedPayload{}
	parser.payloadJsonStore["/api/v1/check_run"] = []api.ParsedPayload{}
	return parser
}

func (fi *PayloadParser) getJsonPayload(route string) ([]api.ParsedPayload, error) {
	payload, ok := fi.payloadJsonStore[route]
	if ok {
		return payload, nil
	}
	return nil, fmt.Errorf("route %s isn't supported", route)
}

func (fi *PayloadParser) IsRouteHandled(route string) bool {
	_, ok := fi.parserMap[route]
	return ok
}

func (fi *PayloadParser) getLogPayLoadJson(payload api.Payload) (interface{}, error) {
	return aggregator.ParseLogPayload(payload)
}

func (fi *PayloadParser) getMetricPayLoadJson(payload api.Payload) (interface{}, error) {
	return aggregator.ParseMetricSeries(payload)
}

func (fi *PayloadParser) getCheckRunPayLoadJson(payload api.Payload) (interface{}, error) {
	return aggregator.ParseCheckRunPayload(payload)
}

func (fi *PayloadParser) parse(payload api.Payload, route string) error {

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
		fi.payloadJsonStore[route] = append(fi.payloadJsonStore[route], parsedPayload)
	}
	return nil
}
