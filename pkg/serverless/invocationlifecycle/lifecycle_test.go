// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
	"github.com/DataDog/datadog-agent/pkg/serverless/proxy"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	"github.com/stretchr/testify/assert"
)

func TestGenerateEnhancedErrorMetricOnInvocationEnd(t *testing.T) {

	d := daemon.StartDaemon("http://localhost:8124")
	defer d.Stop()

	d.ExtraTags.Tags = []string{"functionname:test-function"}

	metricChannel := make(chan []metrics.MetricSample)

	endInvocationTime := time.Now()
	endDetails := proxy.InvocationEndDetails{EndTime: endInvocationTime, IsError: true}

	testProcessor := ProxyProcessor{d.ExtraTags, metricChannel}
	go testProcessor.OnInvokeEnd(&endDetails)

	generatedMetrics := <-metricChannel

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.errors",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       tagset.NewTags(d.ExtraTags.Tags),
		SampleRate: 1,
		Timestamp:  float64(endInvocationTime.UnixNano()),
	}})
}
