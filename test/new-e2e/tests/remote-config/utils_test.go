// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	_ "embed"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/require"
)

// assertLogsWithRetry will verify that a given `agentName` component's logs contain a pattern.
// It will continually retry until the `expectedLogPattern` is found or the `maxRetries` is reached,
// waiting `retryInterval` between each attempt.
// If the `expectedLogPattern` is not found or an error occurs, the calling test will fail.
func assertLogsWithRetry(t *testing.T, rh *components.RemoteHost, agentName string, expectedLogPattern string, maxRetries int, retryInterval time.Duration) {
	err := backoff.Retry(func() error {
		output, err := rh.Execute(fmt.Sprintf("cat /var/log/datadog/%s.log", agentName))
		if err != nil {
			return err
		}
		if strings.Contains(output, expectedLogPattern) {
			return nil
		}
		return errors.New("pattern not found")
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(retryInterval), uint64(maxRetries)))

	if err != nil {
		fmt.Println(rh.MustExecute(fmt.Sprintf("cat /var/log/datadog/%s.log", agentName)))
	}
	require.NoError(t, err, fmt.Sprintf("failed to find log with pattern `%s`", expectedLogPattern))
}

func curlAgentRcServiceWithRetry(t *testing.T, rh *components.RemoteHost, payload string, maxRetries int, retryInterval time.Duration) {
	err := backoff.Retry(func() error {
		_, err := rh.Execute(fmt.Sprintf("curl -sS localhost:8126/v0.7/config -d @- <<EOF\n%sEOF", payload))
		if err != nil {
			return err
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(retryInterval), uint64(maxRetries)))

	require.NoError(t, err, "failed to curl remote config service")
}
