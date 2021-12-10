// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/proxy"
	"github.com/stretchr/testify/assert"
)

func TestGenerateEnhancedErrorMetricOnInvocationEnd(t *testing.T) {

	metricsChan := make(chan []metrics.MetricSample)
	tags := []string{"functionname:test-function"}
	reportLogTime := time.Now()

	endDetails := proxy.InvocationEndDetails{EndTime: reportLogTime, IsError: true}
	testProcessor := ProxyProcessor{tags, metricsChan}

	go testProcessor.OnInvokeEnd(&endDetails)

	generatedMetrics := <-metricsChan

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.errors",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  generatedMetrics[0].Timestamp, //testing against itself due to floating point errors
	}})
}
