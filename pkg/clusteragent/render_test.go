// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package clusteragent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/render"
	"github.com/stretchr/testify/require"
)

func init() {
	render.SetTemplateFolder("../../Dockerfiles/cluster-agent/dist/templates")
}

func TestFormatHPAStatus(t *testing.T) {
	t.Skip()

	tests := map[string]map[string]interface{}{
		"error": {
			"Error": "This is an error",
		},
		"store error": {
			"Cmname":     "datadog-hpa",
			"StoreError": "This is an error",
		},
		"list error": {
			"Cmname": "datadog-hpa",
			"External": map[string]interface{}{
				"ListError": "This is an error",
			},
		},
		"no metrics": {
			"Cmname": "datadog-hpa",
			"External": map[string]interface{}{
				"Total": 2,
				"Valid": 1,
			},
		},
		"one metric": {
			"Cmname": "datadog-hpa",
			"External": map[string]interface{}{
				"Metrics": []custommetrics.ExternalMetricValue{
					{
						MetricName: "metric1",
						Labels:     map[string]string{"foo": "bar"},
						Timestamp:  time.Now().Unix(),
						HPA:        custommetrics.ObjectReference{Name: "hpa", Namespace: "default"},
						Value:      10,
						Valid:      true,
					},
				},
				"Total": 2,
				"Valid": 1,
			},
		},
		"multiple metrics": {
			"Cmname": "datadog-hpa",
			"External": map[string]interface{}{
				"Metrics": []custommetrics.ExternalMetricValue{
					{
						MetricName: "metric1",
						Labels:     map[string]string{"foo": "bar"},
						Timestamp:  time.Now().Unix(),
						HPA:        custommetrics.ObjectReference{Name: "hpa", Namespace: "default"},
						Value:      10,
						Valid:      true,
					},
					{
						MetricName: "metric2",
						Labels:     map[string]string{"foo": "bar"},
						Timestamp:  time.Now().Unix(),
						HPA:        custommetrics.ObjectReference{Name: "hpa", Namespace: "default"},
						Value:      10,
						Valid:      false,
					},
				},
				"Total": 2,
				"Valid": 1,
			},
		},
	}

	for name, status := range tests {
		status := map[string]interface{}{"custommetrics": status}
		b, err := json.Marshal(status)
		require.NoError(t, err)
		out, err := FormatHPAStatus(b)
		require.NoError(t, err)
		t.Logf("Rendering \"%s\"...\n\n%s", name, out)
	}
	t.FailNow()
}
