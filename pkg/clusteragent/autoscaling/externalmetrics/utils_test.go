// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"testing"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/stretchr/testify/assert"
)

func TestMetricNameToDatadogMetricID(t *testing.T) {
	tests := []struct {
		metricName   string
		expID        string
		expParsed    bool
		expHasPrefix bool
	}{
		{
			metricName:   "datadogmetric@myns:name",
			expID:        "myns/name",
			expParsed:    true,
			expHasPrefix: true,
		},
		{
			metricName:   "datadogmetric@name",
			expID:        "",
			expParsed:    false,
			expHasPrefix: true,
		},
		{
			metricName:   "nginx.responsetime",
			expID:        "",
			expParsed:    false,
			expHasPrefix: false,
		},
	}

	for _, test := range tests {
		t.Run(test.metricName, func(t *testing.T) {
			id, parsed, hasPrefix := metricNameToDatadogMetricID(test.metricName)
			assert.Equal(t, test.expID, id)
			assert.Equal(t, test.expParsed, parsed)
			assert.Equal(t, test.expHasPrefix, hasPrefix)
		})
	}
}

func TestDatadogMetricNameGeneration(t *testing.T) {
	testMetricName := "metricName1"
	testLabels := map[string]string{
		"Zlabel1": "foo",
		"Alabel2": "bar",
		"Dlabel3": "baz",
	}

	// Reference
	idRef := getAutogenDatadogMetricName("avg:metricName1{Alabel2:bar,Dlabel3:baz,Zlabel1:foo}.rollup(30)")
	idFromMap := getAutogenDatadogMetricNameFromLabels(testMetricName, testLabels)
	idFromSelector := getAutogenDatadogMetricNameFromSelector(testMetricName, labels.Set(testLabels).AsSelector())

	assert.Equal(t, "dcaautogen-595b170252cd5c77580b802084753c17ed1a18", idRef)
	assert.Equal(t, idRef, idFromMap)
	assert.Equal(t, idRef, idFromSelector)
}

func TestDatadogMetricNameGenerationNoLabels(t *testing.T) {
	testMetricName := "metricName1"
	testLabels := map[string]string{}

	// Reference
	idRef := getAutogenDatadogMetricName("avg:metricName1{*}.rollup(30)")
	idFromMap := getAutogenDatadogMetricNameFromLabels(testMetricName, testLabels)
	idFromSelector := getAutogenDatadogMetricNameFromSelector(testMetricName, labels.Set(testLabels).AsSelector())

	assert.Equal(t, "dcaautogen-cb3c76c6adbd97b438d75e29a6a8efc4cefa81", idRef)
	assert.Equal(t, idRef, idFromMap)
	assert.Equal(t, idRef, idFromSelector)
}

func TestBuildDatadogQueryForExternalMetric(t *testing.T) {
	testMetricName := "metricName1"
	testLabels := map[string]string{
		"Zlabel1": "foo",
		"Alabel2": "bar",
		"Dlabel3": "baz",
	}

	assert.Equal(t, "avg:metricName1{Alabel2:bar,Dlabel3:baz,Zlabel1:foo}.rollup(30)", buildDatadogQueryForExternalMetric(testMetricName, testLabels))

	testLabels = nil
	assert.Equal(t, "avg:metricName1{*}.rollup(30)", buildDatadogQueryForExternalMetric(testMetricName, testLabels))
}
