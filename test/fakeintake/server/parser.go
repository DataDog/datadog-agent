package server

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type PayloadParser struct {
	payloadJsonStore map[string][]api.ParsedPayload
}

func NewPayloadParser() PayloadParser {
	parser := PayloadParser{
		payloadJsonStore: map[string][]api.ParsedPayload{},
	}
	return parser
}

func (fi *PayloadParser) getLogPayLoadData(payload api.Payload) string {
	logs, err := aggregator.ParseLogPayload(payload)
	if err != nil {
		return err.Error()
	}

	output, er := json.Marshal(logs)
	if er != nil {
		return er.Error()
	}
	return string(output)
}

func (fi *PayloadParser) getMetricPayLoadData(payload api.Payload) string {
	MetricOutput, err := aggregator.ParseMetricSeries(payload)
	if err != nil {
		return err.Error()
	}

	output, er := json.Marshal(MetricOutput)
	if er != nil {
		return er.Error()
	}
	return string(output)
}

func (fi *PayloadParser) getCheckRunPayLoadData(payload api.Payload) string {
	MetricOutput, err := aggregator.ParseCheckRunPayload(payload)
	if err != nil {
		return err.Error()
	}

	output, er := json.Marshal(MetricOutput)
	if er != nil {
		return er.Error()
	}
	return string(output)
}

func (fi *PayloadParser) parse(payload api.Payload, route string) {

	parsedPayload := api.ParsedPayload{
		Timestamp: payload.Timestamp,
		Data:      "",
		Encoding:  payload.Encoding,
	}

	if route == "/api/v2/logs" {
		parsedPayload.Data = fi.getLogPayLoadData(payload)
	} else if route == "/api/v2/series" {
		parsedPayload.Data = fi.getMetricPayLoadData(payload)
	} else if route == "/api/v1/check_run" {
		parsedPayload.Data = fi.getCheckRunPayLoadData(payload)
	} else {
		return
	}
	fi.payloadJsonStore[route] = append(fi.payloadJsonStore[route], parsedPayload)
}
