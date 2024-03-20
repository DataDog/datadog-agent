// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func getNewConfig() pkgconfigmodel.ReaderWriter {
	return pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
}

func TestBuildURLShouldReturnHTTPSWithUseSSL(t *testing.T) {
	url := buildURL(config.NewEndpoint("bar", "foo", 0, true))
	assert.Equal(t, "https://foo/v1/input", url)
}

func TestBuildURLShouldReturnHTTPWithoutUseSSL(t *testing.T) {
	url := buildURL(config.NewEndpoint("bar", "foo", 0, false))
	assert.Equal(t, "http://foo/v1/input", url)
}

func TestBuildURLShouldReturnAddressWithPortWhenDefined(t *testing.T) {
	url := buildURL(config.NewEndpoint("bar", "foo", 1234, false))
	assert.Equal(t, "http://foo:1234/v1/input", url)
}

func TestBuildURLShouldReturnAddressForVersion2(t *testing.T) {
	e := config.NewEndpoint("bar", "foo", 0, false)
	e.Version = config.EPIntakeVersion2
	e.TrackType = "test-track"
	url := buildURL(e)
	assert.Equal(t, "http://foo/api/v2/test-track", url)
}

//nolint:revive // TODO(AML) Fix revive linter
func TestDestinationSend200(t *testing.T) {
	cfg := getNewConfig()
	server := NewTestServer(200, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.Destination.Start(input, output, nil)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}
	<-output

	server.Stop()
}

func TestRetries(t *testing.T) {
	// We retry more than just these status codes - testing these to spot check retry works correctly.
	retryTest(t, 500)
	retryTest(t, 429)
	retryTest(t, 404)
}

func TestNoRetries(t *testing.T) {
	testNoRetry(t, 400)
	testNoRetry(t, 401)
	testNoRetry(t, 403)
	testNoRetry(t, 413)
}

//nolint:revive // TODO(AML) Fix revive linter
func testNoRetry(t *testing.T, statusCode int) {
	cfg := getNewConfig()
	server := NewTestServer(statusCode, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.Destination.Start(input, output, nil)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}
	<-output

	// Should not retry this request - no error reported back (because it's not retryable) so input should be unblocked
	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}
	<-output

	server.Stop()
}

func retryTest(t *testing.T, statusCode int) {
	cfg := getNewConfig()
	respondChan := make(chan int)
	server := NewTestServerWithOptions(statusCode, 0, true, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}

	// In a retry loop. let the server respond once
	<-respondChan
	// once it responds a second time, we know `isRetrying` has been set
	<-respondChan
	assert.True(t, <-isRetrying)

	// Should recover because it was retrying
	server.ChangeStatus(200)
	// Drain any retries
	for {
		if (<-respondChan) == 200 {
			break
		}
	}
	<-output

	server.Stop()
}

func TestDestinationContextCancel(t *testing.T) {
	cfg := getNewConfig()
	respondChan := make(chan int)
	server := NewTestServerWithOptions(429, 0, true, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}

	// In a retry loop. let the server respond once
	<-respondChan
	// once it responds a second time, we know `isRetrying` has been set
	<-respondChan
	assert.True(t, <-isRetrying)

	server.Destination.destinationsContext.Stop()

	// If this blocks - the test will timeout and fail. This should not block as the destination context
	// has been canceled and the payload will be dropped. In the real agent, this channel would be closed
	// by the caller while the agent is shutting down
	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}
	server.Stop()
}

func TestConnectivityCheck(t *testing.T) {
	cfg := getNewConfig()
	// Connectivity is ok when server return 200
	server := NewTestServer(200, cfg)
	connectivity := CheckConnectivity(server.Endpoint, cfg)
	assert.Equal(t, config.HTTPConnectivitySuccess, connectivity)
	server.Stop()

	// Connectivity is ok when server return 500
	server = NewTestServer(500, cfg)
	connectivity = CheckConnectivity(server.Endpoint, cfg)
	assert.Equal(t, config.HTTPConnectivityFailure, connectivity)
	server.Stop()
}

func TestErrorToTag(t *testing.T) {
	assert.Equal(t, errorToTag(nil), "none")
	assert.Equal(t, errorToTag(errors.New("fail")), "non-retryable")
	assert.Equal(t, errorToTag(client.NewRetryableError(errors.New("fail"))), "retryable")
}

func TestDestinationSendsV2Protocol(t *testing.T) {
	cfg := getNewConfig()
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()

	server.Destination.protocol = "test-proto"
	err := server.Destination.unconditionalSend(&message.Payload{Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Equal(t, server.request.Header.Get("dd-protocol"), "test-proto")
}

func TestDestinationDoesntSendEmptyV2Protocol(t *testing.T) {
	cfg := getNewConfig()
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()

	err := server.Destination.unconditionalSend(&message.Payload{Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Empty(t, server.request.Header.Values("dd-protocol"))
}

func TestDestinationSendsTimestampHeaders(t *testing.T) {
	cfg := getNewConfig()
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()
	currentTimestamp := time.Now().UnixMilli()

	err := server.Destination.unconditionalSend(&message.Payload{Messages: []*message.Message{{
		IngestionTimestamp: 1234567890_999_999,
	}}, Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Equal(t, server.request.Header.Get("dd-message-timestamp"), "1234567890")

	ddCurrentTimestamp, err := strconv.ParseInt(server.request.Header.Get("dd-current-timestamp"), 10, 64)
	assert.Nil(t, err)
	assert.GreaterOrEqual(t, ddCurrentTimestamp, currentTimestamp)
}

func TestDestinationConcurrentSends(t *testing.T) {
	cfg := getNewConfig()
	// make the server return 500, so the payloads get stuck retrying
	respondChan := make(chan int)
	server := NewTestServerWithOptions(500, 2, true, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload, 10)
	server.Destination.Start(input, output, nil)

	payloads := []*message.Payload{
		// the first two messages will be blocked in concurrent send goroutines
		{Encoded: []byte("a")},
		{Encoded: []byte("b")},
		// the third message will be read out by the main batch sender loop and will be blocked waiting for one of the
		// first two concurrent sends to complete
		{Encoded: []byte("c")},
	}

	for _, p := range payloads {
		input <- p
		<-respondChan
	}

	select {
	case input <- &message.Payload{Encoded: []byte("a")}:
		assert.Fail(t, "should not have been able to write into the channel as the input channel is expected to be backed up due to reaching max concurrent sends")
	default:
	}

	close(input)

	// unblock the destination
	server.ChangeStatus(200)
	// Drain the pending retries
	for {
		if (<-respondChan) == 200 {
			break
		}
	}

	var receivedPayloads []*message.Payload

	for p := range output {
		receivedPayloads = append(receivedPayloads, p)
		if len(receivedPayloads) == len(payloads) {
			break
		}
		<-respondChan
	}

	// order in which messages are received here is not deterministic so compare values
	assert.ElementsMatch(t, payloads, receivedPayloads)
}

// This test ensure the destination's final state is isRetrying = false even if there are pending concurrent sends.
func TestDestinationConcurrentSendsShutdownIsHandled(t *testing.T) {
	cfg := getNewConfig()
	// make the server return 500, so the payloads get stuck retrying
	respondChan := make(chan int)
	server := NewTestServerWithOptions(500, 2, true, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload, 10)

	stopChan := server.Destination.Start(input, output, nil)

	payloads := []*message.Payload{
		{Encoded: []byte("a")},
		{Encoded: []byte("b")},
		{Encoded: []byte("c")},
	}

	for _, p := range payloads {
		input <- p
		<-respondChan
	}
	// trigger shutdown
	close(input)

	// unblock the destination
	server.ChangeStatus(200)
	// Drain the pending retries
	for {
		if (<-respondChan) == 200 {
			break
		}
	}
	// let 2 payloads flow though
	// the first 200 was triggered, collect the output
	<-output
	<-respondChan
	<-output

	select {
	case <-stopChan:
		assert.Fail(t, "Should still be waiting for the last payload to finish")
	default:
	}

	<-respondChan
	<-output
	<-stopChan
	server.Stop()
}

func TestBackoffDelayEnabled(t *testing.T) {
	cfg := getNewConfig()
	respondChan := make(chan int)
	server := NewTestServerWithOptions(500, 0, true, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("test log")}
	<-respondChan
	<-isRetrying

	assert.Equal(t, 1, server.Destination.nbErrors)
	server.Stop()
}

func TestBackoffDelayDisabled(t *testing.T) {
	cfg := getNewConfig()
	respondChan := make(chan int)
	server := NewTestServerWithOptions(500, 0, false, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("test log")}
	<-respondChan

	assert.Equal(t, 0, server.Destination.nbErrors)
	server.Stop()
}

func TestDestinationHA(t *testing.T) {
	variants := []bool{true, false}
	for _, variant := range variants {
		endpoint := config.Endpoint{
			IsHA: variant,
		}
		isEndpointHA := endpoint.IsHA

		dest := NewDestination(endpoint, JSONContentType, client.NewDestinationsContext(), 1, false, "test", getNewConfig())
		isDestHA := dest.IsHA()

		assert.Equal(t, isEndpointHA, isDestHA)
	}
}
