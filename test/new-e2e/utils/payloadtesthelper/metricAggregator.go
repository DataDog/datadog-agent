// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package payloadtesthelper

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io/ioutil"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
)

type MetricAggregator struct {
	metricsByName map[string][]*metricspb.MetricPayload_MetricSeries
}

func NewMetricAggregator() MetricAggregator {
	return MetricAggregator{
		metricsByName: map[string][]*metricspb.MetricPayload_MetricSeries{},
	}
}

func (agg *MetricAggregator) UnmarshallPayloads(body []byte) error {
	response := GetPayloadResponse{}
	json.Unmarshal(body, &response)

	// build filtered metric map
	for _, data := range response.Payloads {
		re, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return err
		}
		enflated, err := ioutil.ReadAll(re)
		if err != nil {
			return err
		}
		metricsPayload := new(metricspb.MetricPayload)
		err = metricsPayload.Unmarshal(enflated)
		if err != nil {
			return err
		}
		for _, serie := range metricsPayload.Series {
			if _, found := agg.metricsByName[serie.Metric]; !found {
				agg.metricsByName[serie.Metric] = []*metricspb.MetricPayload_MetricSeries{}
			}
			agg.metricsByName[serie.Metric] = append(agg.metricsByName[serie.Metric], serie)
		}
	}
	return nil
}

func (agg *MetricAggregator) ContainsMetricName(name string) bool {
	_, found := agg.metricsByName[name]
	return found
}

func (agg *MetricAggregator) ContainsMetricNameAndTags(name string, tags []string) bool {
	series, found := agg.metricsByName[name]
	if !found {
		return false
	}

	for _, serie := range series {
		if areTagsSubsetOfOtherTags(tags, serie.Tags) {
			return true
		}
	}

	return false
}
