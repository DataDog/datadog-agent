// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	_ "embed"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// assertLogsEventually will verify that a given `agentName` component's logs contain a pattern.
// It will continually retry until the `expectedLogPattern` is found or the `maxRetries` is reached,
// waiting `retryInterval` between each attempt.
// If the `expectedLogPattern` is not found or an error occurs, the calling test will fail.
func assertLogsEventually(t *testing.T, rh *components.RemoteHost, agentName string, expectedLogPattern string, waitFor time.Duration, tick time.Duration) {
	t.Helper()
	assert.EventuallyWithTf(t, func(c *assert.CollectT) {
		output, err := rh.Execute(fmt.Sprintf("cat /var/log/datadog/%s.log", agentName))
		if assert.NoError(c, err) {
			assert.Contains(c, output, expectedLogPattern)
		}
	}, waitFor, tick, "failed to find log with pattern `%s`", expectedLogPattern)
}

// mustCurlAgentRcServiceEventually will curl the remote config service's endpoint to get tracer
// configurations every `tick` until either it is successful (in which case it will return the
// output of the curl command), or the `waitFor` duration is reached (in which case it will
// fail the calling test).
func mustCurlAgentRcServiceEventually(t *testing.T, rh *components.RemoteHost, payload string, waitFor time.Duration, tick time.Duration) string {
	t.Helper()
	var output string
	assert.EventuallyWithTf(t, func(c *assert.CollectT) {
		curl, err := rh.Execute(fmt.Sprintf("curl -sS localhost:8126/v0.7/config -d @- <<EOF\n%sEOF", payload))
		assert.NoError(c, err)
		output = curl
	}, waitFor, tick, "could not curl remote config service")
	return output
}
