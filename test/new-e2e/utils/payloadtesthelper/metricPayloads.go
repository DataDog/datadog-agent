package payloadtesthelper

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io/ioutil"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
)

type MetricPayloads struct {
	metricsByName map[string][]*metricspb.MetricPayload_MetricSeries
}

func NewMetricPayloads() MetricPayloads {
	return MetricPayloads{
		metricsByName: map[string][]*metricspb.MetricPayload_MetricSeries{},
	}
}

type GetPayloadResponse struct {
	Payloads [][]byte `json:"payloads"`
}

func (mp *MetricPayloads) UnmarshallPayloads(body []byte) error {
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
			if _, found := mp.metricsByName[serie.Metric]; !found {
				mp.metricsByName[serie.Metric] = []*metricspb.MetricPayload_MetricSeries{}
			}
			mp.metricsByName[serie.Metric] = append(mp.metricsByName[serie.Metric], serie)
		}
	}
	return nil
}

func (mp *MetricPayloads) ContainsMetricName(name string) bool {
	_, found := mp.metricsByName[name]
	return found
}

func (mp *MetricPayloads) ContainsMetricNameAndTags(name string, tags []string) bool {
	series, found := mp.metricsByName[name]
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

func areTagsSubsetOfOtherTags(tags, otherTags []string) bool {
	otherTagsSet := tagsToSet(otherTags)
	for _, tag := range tags {
		if _, found := otherTagsSet[tag]; !found {
			return false
		}
	}
	return true
}

func tagsToSet(tags []string) map[string]struct{} {
	tagsSet := map[string]struct{}{}
	for _, tag := range tags {
		tagsSet[tag] = struct{}{}
	}
	return tagsSet
}
