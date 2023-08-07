package server

import (
	"encoding/json"
	"errors"
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

func (fi *PayloadParser) getJsonPayload(route string) ([]api.ParsedPayload, error) {
	payload, ok := fi.payloadJsonStore[route]
	if ok {
		return payload, nil
	}
	return nil, errors.New("invalid route")
}

func (fi *PayloadParser) getLogPayLoadJson(payload api.Payload) string {
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

func (fi *PayloadParser) getMetricPayLoadJson(payload api.Payload) string {
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

func (fi *PayloadParser) getCheckRunPayLoadJson(payload api.Payload) string {
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
		parsedPayload.Data = fi.getLogPayLoadJson(payload)
	} else if route == "/api/v2/series" {
		parsedPayload.Data = fi.getMetricPayLoadJson(payload)
	} else if route == "/api/v1/check_run" {
		parsedPayload.Data = fi.getCheckRunPayLoadJson(payload)
	} else {
		return
	}
	fi.payloadJsonStore[route] = append(fi.payloadJsonStore[route], parsedPayload)
}
