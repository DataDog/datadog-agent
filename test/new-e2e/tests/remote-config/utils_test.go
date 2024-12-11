// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	_ "embed"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/stretchr/testify/assert"
)

// assertLogsEventually will verify that a given `agentName` component's logs contain a pattern.
// It will continually retry until the `expectedLogPattern` is found or the `maxRetries` is reached,
// waiting `retryInterval` between each attempt.
// If the `expectedLogPattern` is not found or an error occurs, the calling test will fail.
func assertAgentLogsEventually(t *testing.T, rh *components.RemoteHost, agentName string, expectedLogs []string, waitFor time.Duration, tick time.Duration) {
	t.Helper()
	foundLogs := make(map[string]bool, len(expectedLogs))
	missingLogs := make([]string, len(expectedLogs))
	// initially all logs are missing
	copy(missingLogs, expectedLogs)
	remoteLogsPath := fmt.Sprintf("/var/log/datadog/%s.log", agentName)
	t.Logf("looking for logs in %s", remoteLogsPath)
	assert.EventuallyWithTf(t, func(c *assert.CollectT) {
		// read agent logs
		agentLogs, err := readRemoteFile(rh, remoteLogsPath)
		if !assert.NoError(c, err) {
			return
		}
		for _, log := range missingLogs {
			if strings.Contains(agentLogs, log) {
				t.Logf("found log: %s", log)
				foundLogs[log] = true
			}
		}
		// reset missing logs
		missingLogs = make([]string, 0, len(expectedLogs))
		for _, log := range expectedLogs {
			if _, ok := foundLogs[log]; ok {
				continue
			}
			missingLogs = append(missingLogs, log)
		}
		assert.Empty(c, missingLogs, "still missing logs")
		t.Logf("missing logs:\n[%s]", strings.Join(missingLogs, ","))
	}, waitFor, tick, "failed finding logs in agent")
}

// mustCurlAgentRcServiceEventually will curl the remote config service's endpoint to get tracer
// configurations every `tick` until either it is successful (in which case it will return the
// output of the curl command), or the `waitFor` duration is reached (in which case it will
// fail the calling test).
func assertCurlAgentRcServiceContainsEventually(t *testing.T, rh *components.RemoteHost, payload string, expectedKeys []string, waitFor time.Duration, tick time.Duration) string {
	t.Helper()
	var output string
	missingContents := make([]string, len(expectedKeys))
	copy(missingContents, expectedKeys)
	foundContents := make(map[string]bool, len(expectedKeys))
	assert.EventuallyWithTf(t, func(c *assert.CollectT) {
		curlOutput, err := rh.Execute(fmt.Sprintf("curl -sSL localhost:8126/v0.7/config -d @- <<EOF\n%sEOF", payload))
		assert.NoError(c, err)
		for _, content := range missingContents {
			if strings.Contains(curlOutput, content) {
				t.Logf("found content: %s", content)
				foundContents[content] = true
			}
		}
		// reset missing contents
		missingContents = make([]string, 0, len(expectedKeys))
		for _, content := range expectedKeys {
			if _, ok := foundContents[content]; ok {
				continue
			}
			missingContents = append(missingContents, content)
		}
		assert.Empty(c, missingContents, "still missing contents")
	}, waitFor, tick, "could not curl remote config service")
	return output
}

func readRemoteFile(rh *components.RemoteHost, remotePath string) (string, error) {
	localPath := path.Join(os.TempDir(), path.Base(remotePath))
	err := rh.GetFile(remotePath, localPath)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(localPath)
	return string(b), err
}
