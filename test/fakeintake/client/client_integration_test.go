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
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/server"
	"github.com/benbjohnson/clock"
	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	isLocalRun = false
	mockClock  = clock.NewMock()
)

func TestIntegrationClient(t *testing.T) {
	if !isLocalRun {
		t.Skip("skip client integration test on the CI, connection to the server is flaky")
	}
	t.Run("should get empty payloads from a server", func(t *testing.T) {
		ready := make(chan bool, 1)
		fi := server.NewServer(server.WithReadyChannel(ready))
		fi.Start()
		defer fi.Stop()
		isReady := <-ready
		require.True(t, isReady)

		client := NewClient(fi.URL())
		// max wait for 500 ms
		err := backoff.Retry(client.GetServerHealth, backoff.WithMaxRetries(backoff.NewConstantBackOff(100*time.Millisecond), 5))
		require.NoError(t, err, "Failed waiting for fakeintake")

		payloads, err := client.getFakePayloads("/foo/bar")
		assert.NoError(t, err, "Error getting payloads")
		assert.Equal(t, 0, len(payloads))
	})

	t.Run("should get all available payloads from a server on a given endpoint", func(t *testing.T) {
		ready := make(chan bool, 1)
		fi := server.NewServer(server.WithReadyChannel(ready), server.WithClock(mockClock))
		fi.Start()
		defer fi.Stop()
		isReady := <-ready
		require.True(t, isReady)

		// post a test payloads to fakeintake
		// serverUrl := "http://localhost:8080"
		testEndpoint := "/foo/bar"
		resp, err := http.Post(fmt.Sprintf("%s%s", fi.URL(), testEndpoint), "text/plain", strings.NewReader("totoro|5|tag:valid,owner:pducolin"))
		assert.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)

		client := NewClient(fi.URL())
		// max wait for 250 ms
		err = backoff.Retry(client.GetServerHealth, backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Millisecond), 25))
		require.NoError(t, err, "Failed waiting for fakeintake")

		payloads, err := client.getFakePayloads(testEndpoint)
		assert.NoError(t, err, "Error getting payloads")
		assert.Equal(t, 1, len(payloads))
		assert.Equal(t, "totoro|5|tag:valid,owner:pducolin", string(payloads[0].Data))
		assert.Equal(t, "", payloads[0].Encoding)
		assert.Equal(t, mockClock.Now(), payloads[0].Timestamp)
	})
}
