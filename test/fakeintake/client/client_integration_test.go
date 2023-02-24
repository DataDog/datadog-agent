// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/server"
	"github.com/stretchr/testify/assert"
)

var (
	isLocalRun = false
)

func TestIntegrationClient(t *testing.T) {
	if !isLocalRun {
		t.Skip("skip client integration test on the CI, connection to the server is flaky")
	}
	t.Run("should get empty payloads from a server", func(t *testing.T) {
		fi := server.NewServer(8080)
		defer fi.Stop()

		client := NewClient("http://localhost:8080")
		payloads, err := client.getFakePayloads("/foo/bar")
		assert.NoError(t, err, "Error getting payloads")
		assert.Equal(t, 0, len(payloads))
	})

	t.Run("should get all available payloads from a server on a given endpoint", func(t *testing.T) {
		fi := server.NewServer(8080)
		defer fi.Stop()

		// post a test payloads to fakeintake
		serverUrl := "http://localhost:8080"
		testEndpoint := "/foo/bar"
		resp, err := http.Post(fmt.Sprintf("%s%s", serverUrl, testEndpoint), "text/plain", strings.NewReader("totoro|5|tag:valid,owner:pducolin"))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		client := NewClient(serverUrl)
		payloads, err := client.getFakePayloads(testEndpoint)
		assert.NoError(t, err, "Error getting payloads")
		assert.Equal(t, 1, len(payloads))
		assert.Equal(t, "totoro|5|tag:valid,owner:pducolin", string(payloads[0]))
	})
}
