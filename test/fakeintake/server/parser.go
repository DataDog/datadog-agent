// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"errors"
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
	return parser
}

func (fi *PayloadParser) getJsonPayload(route string) ([]api.ParsedPayload, error) {
	payload, ok := fi.payloadJsonStore[route]
	if ok {
		return payload, nil
	}
	return nil, errors.New("invalid route")
}

func (fi *PayloadParser) isValidRoute(route string) bool {
	_, ok := fi.parserMap[route]
	return ok
}

func (fi *PayloadParser) getLogPayLoadJson(payload api.Payload) (interface{}, error) {
	logs, err := aggregator.ParseLogPayload(payload)
	if err != nil {
		return nil, err
	}
	return logs, err
}

func (fi *PayloadParser) getMetricPayLoadJson(payload api.Payload) (interface{}, error) {
	MetricOutput, err := aggregator.ParseMetricSeries(payload)
	if err != nil {
		return nil, err
	}
	return MetricOutput, err
}

func (fi *PayloadParser) getCheckRunPayLoadJson(payload api.Payload) (interface{}, error) {
	CheckOutput, err := aggregator.ParseCheckRunPayload(payload)
	if err != nil {
		return nil, err
	}

	return CheckOutput, err
}

func (fi *PayloadParser) parse(payload api.Payload, route string) {

	parsedPayload := api.ParsedPayload{
		Timestamp: payload.Timestamp,
		Data:      "",
		Encoding:  payload.Encoding,
	}

	if payloadFunc, ok := fi.parserMap[route]; ok {
		parsedPayload.Data, _ = payloadFunc(payload)
	} else {
		return
	}

	fi.payloadJsonStore[route] = append(fi.payloadJsonStore[route], parsedPayload)
}
