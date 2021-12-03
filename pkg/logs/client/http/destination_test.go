// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestBuildURLShouldReturnHTTPSWithUseSSL(t *testing.T) {
	url := buildURL(config.Endpoint{
		APIKey: "bar",
		Host:   "foo",
		UseSSL: true,
	})
	assert.Equal(t, "https://foo/v1/input", url)
}

func TestBuildURLShouldReturnHTTPWithoutUseSSL(t *testing.T) {
	url := buildURL(config.Endpoint{
		APIKey: "bar",
		Host:   "foo",
		UseSSL: false,
	})
	assert.Equal(t, "http://foo/v1/input", url)
}

func TestBuildURLShouldReturnAddressWithPortWhenDefined(t *testing.T) {
	url := buildURL(config.Endpoint{
		APIKey: "bar",
		Host:   "foo",
		Port:   1234,
		UseSSL: false,
	})
	assert.Equal(t, "http://foo:1234/v1/input", url)
}

func TestBuildURLShouldReturnAddressForVersion2(t *testing.T) {
	url := buildURL(config.Endpoint{
		APIKey:    "bar",
		Host:      "foo",
		UseSSL:    false,
		Version:   config.EPIntakeVersion2,
		TrackType: "test-track",
	})
	assert.Equal(t, "http://foo/api/v2/test-track", url)
}

func TestDestinationSend200(t *testing.T) {
	server := NewTestServer(200)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.destination.Start(input, output)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}
	<-output

	server.Stop()
}

func TestDestinationSend500Retries(t *testing.T) {
	server := NewTestServer(500)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.destination.Start(input, output)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}

	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		return server.destination.GetIsRetrying()
	})
	assert.True(t, server.destination.GetIsRetrying())

	// Should recover because it was retrying
	server.ChangeStatus(200)
	<-output

	server.Stop()
}

func TestDestinationSend429Retries(t *testing.T) {
	server := NewTestServer(429)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.destination.Start(input, output)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}

	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		return server.destination.GetIsRetrying()
	})
	assert.True(t, server.destination.GetIsRetrying())

	// Should recover because it was retrying
	server.ChangeStatus(200)
	<-output

	server.Stop()
}

func TestDestinationContextCancel(t *testing.T) {
	server := NewTestServer(429)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.destination.Start(input, output)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}

	testutil.AssertTrueBeforeTimeout(t, 10*time.Millisecond, 1*time.Second, func() bool {
		return server.destination.GetIsRetrying()
	})
	assert.True(t, server.destination.GetIsRetrying())

	server.destination.destinationsContext.Stop()

	// If this blocks - the test will timeout and fail. This should not block as the destination context
	// has been canceled and the payload will be dropped. In the real agent, this channel would be closed
	// by the caller while the agent is shutting down
	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}
	server.Stop()
}

func TestDestinationSend400(t *testing.T) {
	server := NewTestServer(400)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.destination.Start(input, output)

	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}
	<-output

	assert.False(t, server.destination.GetIsRetrying())

	// Should not retry 400 - no error reported back (because it's not retryable) so input should be unblocked
	input <- &message.Payload{Messages: []*message.Message{}, Encoded: []byte("yo")}
	<-output

	assert.False(t, server.destination.GetIsRetrying())
	server.Stop()
}

func TestConnectivityCheck(t *testing.T) {
	// Connectivity is ok when server return 200
	server := NewTestServer(200)
	connectivity := CheckConnectivity(server.Endpoint)
	assert.Equal(t, config.HTTPConnectivitySuccess, connectivity)
	server.Stop()

	// Connectivity is ok when server return 500
	server = NewTestServer(500)
	connectivity = CheckConnectivity(server.Endpoint)
	assert.Equal(t, config.HTTPConnectivityFailure, connectivity)
	server.Stop()
}

func TestErrorToTag(t *testing.T) {
	assert.Equal(t, errorToTag(nil), "none")
	assert.Equal(t, errorToTag(errors.New("fail")), "non-retryable")
	assert.Equal(t, errorToTag(client.NewRetryableError(errors.New("fail"))), "retryable")
}

func TestDestinationSendsV2Protocol(t *testing.T) {
	server := NewTestServer(200)
	defer server.httpServer.Close()

	server.destination.protocol = "test-proto"
	err := server.destination.unconditionalSend(&message.Payload{Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Equal(t, server.request.Header.Get("dd-protocol"), "test-proto")
}

func TestDestinationDoesntSendEmptyV2Protocol(t *testing.T) {
	server := NewTestServer(200)
	defer server.httpServer.Close()

	err := server.destination.unconditionalSend(&message.Payload{Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Empty(t, server.request.Header.Values("dd-protocol"))
}

func TestDestinationConcurrentSends(t *testing.T) {
	// make the server return 500, so the payloads get stuck retrying
	respondChan := make(chan struct{})
	server := NewTestServerWithConcurrency(500, 2, respondChan)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload, 10)

	server.destination.Start(input, output)

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
	// trigger the 200 response to unblock
	<-respondChan

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
