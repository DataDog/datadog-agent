// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver
package custommetrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntake(t *testing.T) {
	tests := []struct {
		desc        string
		payloadjson string
		expected    []PodMetricValue
	}{
		{
			"series with 1 pod metric",
			`{"series":[{"metric":"dd.testing.1","points":[[1417059516,1.0]],"tags":["x:y1","z:zz1","g:k1","tt:1","tz:10", "kube_namespace:default", "kube_pod:dd.test"],"device":"/something/else","type":"gauge","interval":10,"SourceTypeName":"blah","HostTags":["hosta:x","hostb:y","hostc:z","sdfjs:kdsd","eere:s322"]}]}`,
			[]PodMetricValue{
				{
					MetricName: "dd.testing.1",
					PodName:    "dd.test",
					Namespace:  "default",
					Timestamp:  1417059516,
					Value:      1.0,
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			processor := newMockProcessor()
			intake, err := NewIntake(processor)
			require.NoError(t, err)
			intake.Start()
			defer intake.Stop()

			err = intake.Submit([]byte(tt.payloadjson))
			require.NoError(t, err)

			timeout := time.After(1 * time.Second)

			podMetrics := make([]PodMetricValue, 0)

			for {
				select {
				case <-timeout:
					require.Failf(t, "timeout", "got %#v", podMetrics)
				case m := <-processor.sink:
					podMetrics = append(podMetrics, m)
					if len(podMetrics) != len(tt.expected) {
						continue
					}
					assert.ElementsMatch(t, tt.expected, podMetrics)
					return
				}
			}
		})
	}
}
