// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

//go:embed dogstatsdreplay/fixtures/metrics_capture.zstd
var metricsWithTagsCapture []byte

type baseDogstatsdReplaySuite struct {
	e2e.BaseSuite[environments.Host]
}

// uploadCaptureFile uploads a capture file to the remote host
func (v *baseDogstatsdReplaySuite) uploadCaptureFile(captureData []byte, remotePath string) {
	encoded := base64.StdEncoding.EncodeToString(captureData)
	cmd := fmt.Sprintf("echo '%s' | base64 -d > %s", encoded, remotePath)
	v.Env().RemoteHost.MustExecute(cmd)
}

// TestReplayWithTagEnrichment tests that replayed metrics are enriched with tags from tagger state.
func (v *baseDogstatsdReplaySuite) TestReplayWithTagEnrichment() {
	captureFile := "/tmp/metrics_capture.zstd"
	v.uploadCaptureFile(metricsWithTagsCapture, captureFile)

	output := v.Env().RemoteHost.MustExecute(
		"sudo datadog-agent dogstatsd-replay -f " + captureFile)

	assert.Contains(v.T(), output, "replay done")
	assert.NotContains(v.T(), output, "Unable to load state API error")

	// Wait for metrics with tags to arrive at fakeintake. Filter by the
	// statsd-metrics entity so unrelated series captured in the fixture cannot
	// race into metrics[0].
	require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics(
			"custom.metric",
			fakeintakeclient.WithTags[*aggregator.MetricSeries]([]string{
				"container_name:statsd-metrics",
			}),
		)
		assert.NoError(t, err)
		if !assert.NotEmpty(t, metrics, "Expected custom.metric from statsd-metrics to be received") {
			return
		}

		tagString := strings.Join(metrics[0].Tags, ",")
		assert.Contains(t, tagString, "image_name:ghcr.io/datadog/apps-dogstatsd",
			"Expected image_name tag from replay state")
		assert.Contains(t, tagString, "pod_name:statsd-metrics-5d5c7bdc4d-rk88h",
			"Expected pod_name tag from replay state")
	}, 30*time.Second, 1*time.Second, "Intake should have received a fully enriched replay metric.")
}

// TestReplayMissingFile tests error handling when capture file doesn't exist
func (v *baseDogstatsdReplaySuite) TestReplayMissingFile() {
	_, err := v.Env().RemoteHost.Execute(
		"sudo datadog-agent dogstatsd-replay -f /tmp/nonexistent.cap")
	assert.Error(v.T(), err)
}
