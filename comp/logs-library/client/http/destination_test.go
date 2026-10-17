// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"expvar"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBuildURLShouldReturnHTTPSWithUseSSL(t *testing.T) {
	url := buildURL(config.NewEndpoint("bar", "", "foo", 0, config.EmptyPathPrefix, true))
	assert.Equal(t, "https://foo/v1/input", url)
}

func TestBuildURLShouldReturnHTTPWithoutUseSSL(t *testing.T) {
	url := buildURL(config.NewEndpoint("bar", "", "foo", 0, config.EmptyPathPrefix, false))
	assert.Equal(t, "http://foo/v1/input", url)
}

func TestBuildURLShouldReturnAddressWithPortWhenDefined(t *testing.T) {
	url := buildURL(config.NewEndpoint("bar", "", "foo", 1234, config.EmptyPathPrefix, false))
	assert.Equal(t, "http://foo:1234/v1/input", url)
}

func TestBuildURLShouldReturnAddressForVersion2(t *testing.T) {
	e := config.NewEndpoint("bar", "", "foo", 0, config.EmptyPathPrefix, false)
	e.Version = config.EPIntakeVersion2
	e.TrackType = "test-track"
	url := buildURL(e)
	assert.Equal(t, "http://foo/api/v2/test-track", url)
}

func TestBuildURLPathPrefix(t *testing.T) {
	e := config.NewEndpoint("bar", "", "foo", 0, "/prefix/url", false)
	e.Version = config.EPIntakeVersion2
	e.TrackType = "test-track"
	url := buildURL(e)
	assert.Equal(t, "http://foo/prefix/url/api/v2/test-track", url)
}

func TestBuildURLPathPrefixSSLPort(t *testing.T) {
	e := config.NewEndpoint("bar", "", "foo", 8080, "/prefix/url", true)
	e.Version = config.EPIntakeVersion2
	e.TrackType = "test-track"
	url := buildURL(e)
	assert.Equal(t, "https://foo:8080/prefix/url/api/v2/test-track", url)
}

func TestBuildURLPathPrefixV1(t *testing.T) {
	e := config.NewEndpoint("bar", "", "foo", 8080, "/prefix/url", true)
	e.Version = config.EPIntakeVersion1
	e.TrackType = "test-track"
	url := buildURL(e)
	assert.Equal(t, "https://foo:8080/prefix/url/v1/input", url)
}

func TestBuildURLIPv6BareHost(t *testing.T) {
	url := buildURL(config.NewEndpoint("bar", "", "fd38::1", 8080, config.EmptyPathPrefix, true))
	assert.Equal(t, "https://[fd38::1]:8080/v1/input", url)
}

func TestBuildURLIPv6BracketedHost(t *testing.T) {
	url := buildURL(config.NewEndpoint("bar", "", "[fd38::1]", 8080, config.EmptyPathPrefix, true))
	assert.Equal(t, "https://[fd38::1]:8080/v1/input", url)
}

func TestDestinationSend200(t *testing.T) {
	cfg := configmock.New(t)
	server := NewTestServer(200, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.Destination.Start(input, output, nil)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}
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

func testNoRetry(t *testing.T, statusCode int) {
	cfg := configmock.New(t)
	server := NewTestServer(statusCode, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.Destination.Start(input, output, nil)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}
	<-output

	// Should not retry this request - no error reported back (because it's not retryable) so input should be unblocked
	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}
	<-output

	server.Stop()
}

func TestLogsDroppedMetric(t *testing.T) {
	testLogsDropped(t, 400)
	testLogsDropped(t, 401)
	testLogsDropped(t, 403)
	testLogsDropped(t, 413)
}

func testLogsDropped(t *testing.T, statusCode int) {
	cfg := configmock.New(t)
	telemetryMock := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	metrics.TlmLogsDropped = telemetryMock.NewCounter("logs", "dropped", []string{"destination"}, "")

	server := NewTestServer(statusCode, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	server.Destination.Start(input, output, nil)

	payload := &message.Payload{
		MessageMetas: []*message.MessageMetadata{
			{},
			{},
			{},
		},
		Encoded: []byte("test payload"),
	}

	// Send Payload that should fail & be non-retryable
	input <- payload
	<-output

	// Verify the logs.dropped metric was incremented & has correct destination tag
	metric, err := telemetryMock.(telemetry.Mock).GetCountMetric("logs", "dropped")
	assert.NoError(t, err)
	assert.Len(t, metric, 1, "Should have one metric entry")

	assert.Equal(t, float64(3), metric[0].Value())
	assert.Equal(t, server.Destination.host, metric[0].Tags()["destination"])

	server.Stop()
}

func retryTest(t *testing.T, statusCode int) {
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(statusCode, 1, true, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}

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

// mockSecrets wraps the noop secrets implementation to track Refresh calls
// and control IsValueFromSecret behavior.
type mockSecrets struct {
	secretnooptypes.SecretNoop
	refreshCount    atomic.Int32
	valueFromSecret bool
}

func (m *mockSecrets) Refresh() bool {
	m.refreshCount.Add(1)
	return true
}

func (m *mockSecrets) IsValueFromSecret(_ string) bool {
	return m.valueFromSecret
}

func TestForbiddenTriggersSecretsRefreshAndRetry(t *testing.T) {
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(403, 1, true, respondChan, cfg)

	mock := &mockSecrets{valueFromSecret: true}
	server.Destination.secrets = mock

	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}

	<-respondChan
	<-respondChan
	assert.True(t, <-isRetrying)
	assert.GreaterOrEqual(t, mock.refreshCount.Load(), int32(1), "secrets.Refresh should have been called on 403")

	server.ChangeStatus(200)
	for {
		if (<-respondChan) == 200 {
			break
		}
	}
	<-output

	server.Stop()
}

func TestForbiddenDropsWhenKeyNotFromSecret(t *testing.T) {
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(403, 1, true, respondChan, cfg)

	mock := &mockSecrets{valueFromSecret: false}
	server.Destination.secrets = mock

	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}

	<-respondChan
	assert.Equal(t, int32(0), mock.refreshCount.Load(), "secrets.Refresh should NOT have been called when key is not from secret")

	server.Stop()
}

func TestDestinationContextCancel(t *testing.T) {
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(429, 1, true, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}

	// In a retry loop. let the server respond once
	<-respondChan
	// once it responds a second time, we know `isRetrying` has been set
	<-respondChan
	assert.True(t, <-isRetrying)

	server.Destination.destinationsContext.Stop()

	// If this blocks - the test will timeout and fail. This should not block as the destination context
	// has been canceled and the payload will be dropped. In the real agent, this channel would be closed
	// by the caller while the agent is shutting down
	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}
	server.Stop()
}

// TestDestinationErrorLogFormat tests that the error log format is correct
func TestDestinationErrorLogFormat(t *testing.T) {
	cfg := configmock.New(t)

	// Create a server that returns 403 to trigger the error log
	server := NewTestServer(403, cfg)
	defer server.httpServer.Close()

	// Set up destination with all metadata populated
	server.Destination.destMeta = client.NewDestinationMetadata("dbm-samples", "1", "reliable", "0", "DBM")
	server.Destination.protocol = "test-protocol"
	server.Destination.origin = "test-origin"
	server.Destination.endpoint.TrackType = "test-track"

	// Capture log output
	var logOutput bytes.Buffer
	writer := bufio.NewWriter(&logOutput)

	// Set up logger to capture log output
	testLogger, err := log.LoggerFromWriterWithMinLevelAndMsgFormat(writer, log.WarnLvl)
	assert.NoError(t, err)
	log.SetupLogger(testLogger, "warn")

	// Send payload to trigger error log
	err = server.Destination.unconditionalSend(&message.Payload{
		Encoded:  []byte("test payload"),
		Encoding: "gzip",
	})

	assert.Error(t, err)
	writer.Flush()

	// Verify log contains expected metadata
	logStr := logOutput.String()
	t.Logf("Captured log output: %s", logStr)

	// Check that log contains key parts of enhanced metadata
	assert.True(t, strings.Contains(logStr, "code=403"), "Log should contain status code")
	assert.True(t, strings.Contains(logStr, "url="), "Log should contain URL")
	assert.True(t, strings.Contains(logStr, "EvP track type=test-track"), "Log should contain track type")
	assert.True(t, strings.Contains(logStr, "EvP category=DBM"), "Log should contain category")
	assert.True(t, strings.Contains(logStr, "origin=test-origin"), "Log should contain origin")
	assert.True(t, strings.Contains(logStr, "content type=application/json"), "Log should contain content type")
}

func TestConnectivityCheck(t *testing.T) {
	cfg := configmock.New(t)
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
	cfg := configmock.New(t)
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()

	server.Destination.protocol = "test-proto"
	err := server.Destination.unconditionalSend(&message.Payload{Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Equal(t, server.request.Header.Get("dd-protocol"), "test-proto")
}

func TestDestinationDoesntSendEmptyV2Protocol(t *testing.T) {
	cfg := configmock.New(t)
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()

	err := server.Destination.unconditionalSend(&message.Payload{Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Empty(t, server.request.Header.Values("dd-protocol"))
}

func TestDestinationSendsTimestampHeaders(t *testing.T) {
	cfg := configmock.New(t)
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()
	currentTimestamp := time.Now().UnixMilli()

	err := server.Destination.unconditionalSend(&message.Payload{MessageMetas: []*message.MessageMetadata{
		{
			IngestionTimestamp: 9234567890999999,
		},
		{
			IngestionTimestamp: 1234567890999999,
		},
	}, Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Equal(t, server.request.Header.Get("dd-message-timestamp"), "1234567890")

	ddCurrentTimestamp, err := strconv.ParseInt(server.request.Header.Get("dd-current-timestamp"), 10, 64)
	assert.Nil(t, err)
	assert.GreaterOrEqual(t, ddCurrentTimestamp, currentTimestamp)
}

func TestDestinationSendsUserAgent(t *testing.T) {
	cfg := configmock.New(t)
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()

	err := server.Destination.unconditionalSend(&message.Payload{Encoded: []byte("payload")})
	assert.Nil(t, err)
	assert.Regexp(t, regexp.MustCompile("datadog-agent/.*"), server.request.Header.Values("user-agent"))
}

func TestDestinationConcurrentSends(t *testing.T) {
	cfg := configmock.New(t)
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
	cfg := configmock.New(t)
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
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(500, 1, true, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
	<-respondChan
	<-isRetrying

	assert.Equal(t, 1, server.Destination.nbErrors)
	server.Stop()
}

func TestBackoffDelayDisabled(t *testing.T) {
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(500, 1, false, respondChan, cfg)
	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
	<-respondChan

	assert.Equal(t, 0, server.Destination.nbErrors)
	server.Stop()
}

func TestPendingDelegatedAuthDoesNotSend(t *testing.T) {
	cfg := configmock.New(t)
	server := NewTestServer(http.StatusOK, cfg)
	defer server.Stop()

	server.Destination.endpoint = config.NewEndpoint(
		"DELA[some-org-uuid, aws]",
		"logs_config.additional_endpoints",
		server.Endpoint.Host,
		server.Endpoint.Port,
		config.EmptyPathPrefix,
		false,
	)

	err := server.Destination.unconditionalSend(&message.Payload{
		MessageMetas: []*message.MessageMetadata{},
		Encoded:      []byte("test log"),
	})

	assert.ErrorIs(t, err, errCredentialNotReady)
	assert.Empty(t, server.request.Header)
}

func TestPendingDelegatedAuthHoldsPayloadWithoutAcking(t *testing.T) {
	cfg := configmock.New(t)
	server := NewDelegatedAuthPendingTestServer(cfg)
	defer server.Stop()

	prev := credentialWaitInterval
	credentialWaitInterval = 10 * time.Millisecond
	t.Cleanup(func() { credentialWaitInterval = prev })

	errorsBefore := metrics.DestinationErrors.Value()
	payload := &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
	output := make(chan *message.Payload, 1)
	isRetrying := make(chan bool, 1)
	done := make(chan destinationResult, 1)
	go func() { done <- server.Destination.sendAndRetry(payload, output, isRetrying) }()

	// Reported as retrying so the worker skips it, but the payload is never acked.
	select {
	case retrying := <-isRetrying:
		assert.True(t, retrying)
	case <-time.After(5 * time.Second):
		t.Fatal("destination never reported retrying")
	}
	select {
	case <-output:
		t.Fatal("payload acked without a credential")
	case <-time.After(100 * time.Millisecond):
	}
	server.Destination.retryLock.Lock()
	assert.Zero(t, server.Destination.nbErrors, "waiting for a credential must not grow the backoff")
	server.Destination.retryLock.Unlock()
	assert.Equal(t, errorsBefore, metrics.DestinationErrors.Value(), "waiting for a credential is not a network error")

	// Shutdown ends the wait instead of retrying forever.
	server.DestCtx.Stop()
	select {
	case result := <-done:
		assert.ErrorIs(t, result.err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("sendAndRetry kept retrying after shutdown")
	}
	assert.Empty(t, output)
	assert.Empty(t, server.request.Header)
}

func TestPendingDelegatedAuthSendsQueuedPayloadOnceKeyArrives(t *testing.T) {
	cfg := configmock.New(t)
	server := NewTestServerWithOptions(http.StatusOK, 1, true, nil, cfg)
	defer server.Stop()

	prev := credentialWaitInterval
	credentialWaitInterval = 10 * time.Millisecond
	t.Cleanup(func() { credentialWaitInterval = prev })

	entry := func(apiKey string) []map[string]interface{} {
		return []map[string]interface{}{{
			"api_key": apiKey, "host": server.Endpoint.Host, "port": server.Endpoint.Port, "use_ssl": false,
		}}
	}
	cfg.SetInTest("logs_config.force_use_http", true)
	cfg.SetInTest("logs_config.additional_endpoints", entry("DELA[some-org-uuid, aws]"))
	endpoints, err := config.BuildHTTPEndpointsWithConfig(cfg, config.NewLogsConfigKeys("logs_config.", cfg), "agent-http-intake.logs.", "", "", "")
	require.NoError(t, err)
	require.Len(t, endpoints.Endpoints, 2)
	pending := endpoints.Endpoints[1]
	require.True(t, pending.AwaitsDelegatedAuthKey())
	// Shares the credential with the config-built endpoint, so the write-back below reaches it.
	server.Destination.endpoint = pending

	payload := &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
	output := make(chan *message.Payload, 1)
	isRetrying := make(chan bool, 2)
	go server.Destination.sendAndRetry(payload, output, isRetrying)
	require.True(t, <-isRetrying)

	// Delegated auth writes the resolved key back into config.
	cfg.Set("logs_config.additional_endpoints", entry("resolved-key"), pkgconfigmodel.SourceSecret)

	select {
	case sent := <-output:
		assert.Same(t, payload, sent)
	case <-time.After(5 * time.Second):
		t.Fatal("queued payload not sent after the key arrived")
	}
	assert.False(t, <-isRetrying, "retry state clears once the queued payload is sent")
	assert.Equal(t, "resolved-key", server.request.Header.Get("DD-API-KEY"))
}

func TestDelegatedAuthWithoutWriteBackDropsWithoutAcking(t *testing.T) {
	cfg := configmock.New(t)
	server := NewDelegatedAuthPendingTestServer(cfg)
	defer server.Stop()
	// A directive in a setting delegated auth never writes back: no key will ever arrive.
	server.Destination.endpoint = config.NewEndpoint(
		"DELA[some-org-uuid, aws]",
		"database_monitoring.samples.additional_endpoints",
		server.Endpoint.Host,
		server.Endpoint.Port,
		config.EmptyPathPrefix,
		false,
	)

	errorsBefore := metrics.DestinationErrors.Value()
	payload := &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
	output := make(chan *message.Payload, 1)
	isRetrying := make(chan bool, 1)
	done := make(chan destinationResult, 1)
	go func() { done <- server.Destination.sendAndRetry(payload, output, isRetrying) }()

	// Returns promptly instead of holding the payload, reports retrying, and never acks.
	select {
	case result := <-done:
		assert.NoError(t, result.err)
	case <-time.After(5 * time.Second):
		t.Fatal("held a payload no key will ever be written back for")
	}
	assert.True(t, <-isRetrying)
	assert.Empty(t, output)
	assert.Equal(t, errorsBefore, metrics.DestinationErrors.Value())
	assert.Empty(t, server.request.Header)
}

func TestPendingDelegatedAuthDropsUnreliableCopy(t *testing.T) {
	cfg := configmock.New(t)
	server := NewDelegatedAuthPendingTestServer(cfg)
	defer server.Stop()
	server.Destination.shouldRetry = false

	errorsBefore := metrics.DestinationErrors.Value()
	payload := &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
	output := make(chan *message.Payload, 1)

	result := server.Destination.sendAndRetry(payload, output, make(chan bool, 1))

	// An unreliable destination's output doesn't reach the auditor, so its copy is dropped.
	assert.NoError(t, result.err)
	assert.Same(t, payload, <-output)
	assert.Equal(t, errorsBefore, metrics.DestinationErrors.Value())
	assert.Empty(t, server.request.Header)
}

func TestDestinationHA(t *testing.T) {
	variants := []bool{true, false}
	for _, variant := range variants {
		endpoint := config.Endpoint{
			IsMRF: variant,
		}
		isEndpointMRF := endpoint.IsMRF

		dest := NewDestination(endpoint, JSONContentType, client.NewDestinationsContext(), false, client.NewNoopDestinationMetadata(), configmock.New(t), 1, 1, metrics.NewNoopPipelineMonitor(""), "test", secretsnoopimpl.NewComponent().Comp)
		isDestMRF := dest.IsMRF()

		assert.Equal(t, isEndpointMRF, isDestMRF)
	}
}

func TestTransportProtocol_HTTP1(t *testing.T) {
	c := configmock.New(t)
	assert.True(t, c.IsKnown("logs_config.http_protocol"), "Config key logs_config.http_protocol should be known")

	// Force client to use HTTP/1
	c.SetInTest("logs_config.http_protocol", "http1")
	// Skip SSL validation
	c.SetInTest("skip_ssl_validation", true)

	s := NewTestHTTPSServer(false)
	defer s.Close()

	c.SetInTest("logs_config.http_timeout", 5)
	client := httpClientFactory(c, NoTimeoutOverride)()

	assert.Equal(t, 5*time.Second, client.Timeout)
	// Create an HTTP/1.1 request
	req, err := http.NewRequest("POST", s.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Client send an HTTP1 request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Assert the protocol is HTTP/1.1
	assert.Equal(t, "HTTP/1.1", resp.Proto)
}

func TestTransportProtocol_HTTP2(t *testing.T) {
	c := configmock.New(t)
	assert.True(t, c.IsKnown("logs_config.http_protocol"), "Config key logs_config.http_protocol should be known")

	// Force client to use ALNP
	c.SetInTest("logs_config.http_protocol", "auto")
	// Skip SSL validation
	c.SetInTest("skip_ssl_validation", true)

	s := NewTestHTTPSServer(false)
	defer s.Close()

	c.SetInTest("logs_config.http_timeout", 5)
	client := httpClientFactory(c, NoTimeoutOverride)()

	assert.Equal(t, 5*time.Second, client.Timeout)
	req, err := http.NewRequest("POST", s.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Client send an HTTP/2 request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Assert the protocol is HTTP/2.0
	assert.Equal(t, "HTTP/2.0", resp.Proto)
}

func TestTransportProtocol_InvalidProtocol(t *testing.T) {
	c := configmock.New(t)
	assert.True(t, c.IsKnown("logs_config.http_protocol"), "Config key logs_config.http_protocol should be known")

	// Force client to default to ALNP from invalid protocol
	c.SetInTest("logs_config.http_protocol", "htto2")
	// Skip SSL validation
	c.SetInTest("skip_ssl_validation", true)

	// Start the test server
	server := NewTestHTTPSServer(false)
	defer server.Close()

	c.SetInTest("logs_config.http_timeout", 5)
	client := httpClientFactory(c, NoTimeoutOverride)()

	assert.Equal(t, 5*time.Second, client.Timeout)
	req, err := http.NewRequest("POST", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	// Client send an HTTP/1.1 request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Assert that the server responds with best available protocol(http/2.0)
	assert.Equal(t, "HTTP/2.0", resp.Proto)
}

func TestTransportProtocol_HTTP1FallBack(t *testing.T) {
	c := configmock.New(t)
	assert.True(t, c.IsKnown("logs_config.http_protocol"), "Config key logs_config.http_protocol should be known")

	// Force client to use ALNP
	c.SetInTest("logs_config.http_protocol", "auto")
	// Skip SSL validation
	c.SetInTest("skip_ssl_validation", true)

	// Start the test server that only support HTTP/1.1
	server := NewTestHTTPSServer(true)
	defer server.Close()

	c.SetInTest("logs_config.http_timeout", 5)
	client := httpClientFactory(c, NoTimeoutOverride)()

	assert.Equal(t, 5*time.Second, client.Timeout)
	req, err := http.NewRequest("POST", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	// Client send HTTP/2 request
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Assert that the server automatically falls back to HTTP/1.1
	assert.Equal(t, "HTTP/1.1", resp.Proto)
}

func TestTransportProtocol_HTTP2WhenUsingProxy(t *testing.T) {
	c := configmock.New(t)

	// Force client to use ALNP
	c.SetInTest("logs_config.http_protocol", "auto")
	c.SetInTest("skip_ssl_validation", true)

	// The test server uses TLS, so if we set the http proxy (not https), it still makes
	// a request to the test server
	c.SetInTest("proxy.http", "http://foo.bar")

	server := NewTestHTTPSServer(false)
	defer server.Close()

	c.SetInTest("logs_config.http_timeout", 5)
	client := httpClientFactory(c, NoTimeoutOverride)()

	assert.Equal(t, 5*time.Second, client.Timeout)
	req, err := http.NewRequest("POST", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Assert that the server chose HTTP/2.0 because a proxy was configured
	assert.Equal(t, "HTTP/2.0", resp.Proto)
}

func TestTransportProtocol_HTTP1FallBackWhenUsingProxy(t *testing.T) {
	c := configmock.New(t)

	// Force client to use ALNP
	c.SetInTest("logs_config.http_protocol", "auto")
	c.SetInTest("skip_ssl_validation", true)

	// The test server uses TLS, so if we set the http proxy (not https), it still makes
	// a request to the test server
	c.SetInTest("proxy.http", "http://foo.bar")

	// Start the test server that only support HTTP/1.1
	server := NewTestHTTPSServer(true)
	defer server.Close()

	c.SetInTest("logs_config.http_timeout", 5)
	client := httpClientFactory(c, NoTimeoutOverride)()

	assert.Equal(t, 5*time.Second, client.Timeout)
	req, err := http.NewRequest("POST", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Assert that the server chose HTTP/1.1 because a proxy was configured
	assert.Equal(t, "HTTP/1.1", resp.Proto)
}

// TestDestinationSourceTagBasedOnTelemetryName tests that the source tag is set when the telemetry name contains "logs" source tag
func TestDestinationSourceTagBasedOnTelemetryName(t *testing.T) {
	cfg := configmock.New(t)

	// Create telemetry mock
	telemetryMock := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	metrics.TlmBytesSent = telemetryMock.NewCounter("logs", "bytes_sent", []string{"emitter", "source"}, "")
	metrics.TlmEncodedBytesSent = telemetryMock.NewCounter("logs", "encoded_bytes_sent", []string{"emitter", "source", "compression_kind"}, "")

	// Create a new server
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()

	// Test case: Telemetry name contains "logs" -> sourceTag should be "logs"
	server.Destination.destMeta = client.NewDestinationMetadata("logs", "3", "reliable", "0", "")
	payload := &message.Payload{
		Encoded:       []byte("payload"),
		UnencodedSize: 7, // len("payload")
	}

	// Send the payload to the server
	err := server.Destination.unconditionalSend(payload)
	assert.Nil(t, err)
	assert.Equal(t, "logs_3_reliable_0", server.Destination.destMeta.TelemetryName())

	// Verify the source tag is "logs" in the telemetry metric
	metric, err := telemetryMock.(telemetry.Mock).GetCountMetric("logs", "bytes_sent")
	assert.NoError(t, err)
	assert.Len(t, metric, 1)
	assert.Equal(t, "agent", metric[0].Tags()["emitter"]) // "agent" for core agent (via GetAgentIdentityTag)
	assert.Equal(t, "logs", metric[0].Tags()["source"])

	metric, err = telemetryMock.(telemetry.Mock).GetCountMetric("logs", "encoded_bytes_sent")
	assert.NoError(t, err)
	assert.Len(t, metric, 1)
	assert.Equal(t, "agent", metric[0].Tags()["emitter"])
	assert.Equal(t, "logs", metric[0].Tags()["source"])
}

// TestDestinationSourceTagEPForwarder tests that the source tag is set to "epforwarder" when the telemetry source name does not contain "logs"
func TestDestinationSourceTagEPForwarder(t *testing.T) {
	cfg := configmock.New(t)

	// Create telemetry mock
	telemetryMock := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	metrics.TlmBytesSent = telemetryMock.NewCounter("logs", "bytes_sent", []string{"emitter", "source"}, "")
	metrics.TlmEncodedBytesSent = telemetryMock.NewCounter("logs", "encoded_bytes_sent", []string{"emitter", "source", "compression_kind"}, "")

	// Create a new server
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()

	// Test case: Telemetry name does not contain "logs" -> sourceTag should be "epforwarder"
	server.Destination.destMeta = client.NewDestinationMetadata("dbm", "1", "reliable", "0", "")
	payload := &message.Payload{
		Encoded:       []byte("payload"),
		UnencodedSize: 7, // len("payload")
	}

	err := server.Destination.unconditionalSend(payload)
	assert.Nil(t, err)
	assert.Equal(t, "dbm_1_reliable_0", server.Destination.destMeta.TelemetryName())

	// Verify the source tag is "epforwarder" in the telemetry metric
	metric, err := telemetryMock.(telemetry.Mock).GetCountMetric("logs", "bytes_sent")
	assert.NoError(t, err)
	assert.Len(t, metric, 1)
	assert.Equal(t, "agent", metric[0].Tags()["emitter"]) // "agent" for core agent (via GetAgentIdentityTag)
	assert.Equal(t, "epforwarder", metric[0].Tags()["source"])

	metric, err = telemetryMock.(telemetry.Mock).GetCountMetric("logs", "encoded_bytes_sent")
	assert.NoError(t, err)
	assert.Len(t, metric, 1)
	assert.Equal(t, "agent", metric[0].Tags()["emitter"])
	assert.Equal(t, "epforwarder", metric[0].Tags()["source"])
}

// TestDestinationCompression tests what the compression kind is set when compression is used
func TestDestinationCompression(t *testing.T) {
	cfg := configmock.New(t)

	// Create telemetry mock
	telemetryMock := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	metrics.TlmEncodedBytesSent = telemetryMock.NewCounter("logs", "encoded_bytes_sent", []string{"emitter", "source", "compression_kind"}, "")

	// Create a new server with compression enabled
	server := NewTestServer(200, cfg)
	defer server.httpServer.Close()

	// Enable compression and set zstdcompression kind
	server.Destination.endpoint.UseCompression = true
	server.Destination.endpoint.CompressionKind = "zstd"

	// Test case 1: Telemetry uses zstd compression
	server.Destination.destMeta = client.NewDestinationMetadata("dbm", "1", "reliable", "0", "")
	payload := &message.Payload{
		Encoded:       []byte("payload"),
		UnencodedSize: 7, // len("payload")
	}

	err := server.Destination.unconditionalSend(payload)
	assert.Nil(t, err)
	assert.Equal(t, "dbm_1_reliable_0", server.Destination.destMeta.TelemetryName())

	// Verify the compression tag is set correctly
	metric, err := telemetryMock.(telemetry.Mock).GetCountMetric("logs", "encoded_bytes_sent")
	assert.NoError(t, err)
	assert.Len(t, metric, 1)
	assert.Equal(t, "zstd", metric[0].Tags()["compression_kind"])

	// Test case 2: Telemetry uses gzip compression
	// Enable compression and set gzip compression kind
	server.Destination.endpoint.CompressionKind = "gzip"

	server.Destination.destMeta = client.NewDestinationMetadata("dbm", "2", "reliable", "0", "")
	payload2 := &message.Payload{
		Encoded:       []byte("payload"),
		UnencodedSize: 7,
	}

	err = server.Destination.unconditionalSend(payload2)
	assert.Nil(t, err)
	assert.Equal(t, "dbm_2_reliable_0", server.Destination.destMeta.TelemetryName())

	// Verify the compression tag is set correctly
	metric, err = telemetryMock.(telemetry.Mock).GetCountMetric("logs", "encoded_bytes_sent")
	assert.NoError(t, err)
	assert.Len(t, metric, 2)
	assert.Equal(t, "gzip", metric[0].Tags()["compression_kind"])
}

func TestHTTPTimeoutOverride(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.http_timeout", 1)
	client := httpClientFactory(cfg, 15*time.Second)()
	assert.Equal(t, 15*time.Second, client.Timeout)
}

// TestClassifyConnectivityError verifies that classifyConnectivityError maps
// concrete network error types to the expected stable failure_cause tag values.
func TestClassifyConnectivityError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error returns empty string",
			err:      nil,
			expected: "",
		},
		{
			name:     "dns error",
			err:      &url.Error{Op: "Post", URL: "https://example.com", Err: &net.DNSError{Err: "no such host", Name: "example.com"}},
			expected: metrics.FailureCauseDNS,
		},
		{
			name: "dns error wrapped in RetryableError",
			err: client.NewRetryableError(&url.Error{
				Op:  "Post",
				URL: "https://example.com",
				Err: &net.DNSError{Err: "no such host", Name: "example.com"},
			}),
			expected: metrics.FailureCauseDNS,
		},
		{
			name:     "timeout error",
			err:      &url.Error{Op: "Post", URL: "https://example.com", Err: &net.DNSError{Err: "i/o timeout", Name: "example.com", IsTimeout: true}},
			expected: metrics.FailureCauseTimeout,
		},
		{
			name:     "tcp connection refused",
			err:      &url.Error{Op: "Post", URL: "https://example.com", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}},
			expected: metrics.FailureCauseConnection,
		},
		{
			name:     "tls alert error",
			err:      &url.Error{Op: "Post", URL: "https://example.com", Err: tls.AlertError(42)},
			expected: metrics.FailureCauseTLS,
		},
		{
			name:     "tls string in error message",
			err:      &url.Error{Op: "Post", URL: "https://example.com", Err: errors.New("tls: bad certificate")},
			expected: metrics.FailureCauseTLS,
		},
		{
			name:     "x509 certificate error",
			err:      &url.Error{Op: "Post", URL: "https://example.com", Err: errors.New("x509: certificate signed by unknown authority")},
			expected: metrics.FailureCauseTLS,
		},
		{
			name:     "http_status for errClient sentinel",
			err:      errClient,
			expected: metrics.FailureCauseHTTPStatus,
		},
		{
			name:     "http_status for errServer sentinel",
			err:      errServer,
			expected: metrics.FailureCauseHTTPStatus,
		},
		{
			// This is the boot-time 5xx / 403-refresh path: unconditionalSend wraps
			// errServer in a RetryableError. Before adding Unwrap() to RetryableError
			// the classifier missed this and returned "other".
			name:     "http_status for errServer wrapped in RetryableError",
			err:      client.NewRetryableError(errServer),
			expected: metrics.FailureCauseHTTPStatus,
		},
		{
			name:     "http_status for errClient wrapped in RetryableError",
			err:      client.NewRetryableError(errClient),
			expected: metrics.FailureCauseHTTPStatus,
		},
		{
			// url.Error wrapped in RetryableError — relies on errors.As traversal,
			// which now works because RetryableError implements Unwrap.
			name: "dns for url.Error wrapped in RetryableError",
			err: client.NewRetryableError(&url.Error{
				Op:  "Post",
				URL: "https://example.com",
				Err: &net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true},
			}),
			expected: metrics.FailureCauseDNS,
		},
		{
			name:     "other for unknown error",
			err:      errors.New("something went wrong"),
			expected: metrics.FailureCauseOther,
		},
		{
			name:     "mixed-case TLS message classified correctly",
			err:      errors.New("TLS handshake failure"),
			expected: metrics.FailureCauseTLS,
		},
		{
			name:     "mixed-case DNS message classified correctly",
			err:      errors.New("No Such Host"),
			expected: metrics.FailureCauseDNS,
		},
		{
			// Malformed *url.Error{Err: nil} from a third-party transport must not panic.
			name:     "other for url.Error with nil inner",
			err:      &url.Error{Op: "Post", URL: "https://example.com", Err: nil},
			expected: metrics.FailureCauseOther,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyConnectivityError(tc.err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

// TestCheckConnectivityIncrementsFailureMetric verifies that CheckConnectivity wires
// the classifier's result into TlmHTTPConnectivityFailure.Inc.  A destination whose
// host cannot be resolved is guaranteed to produce a DNS error, so we can assert that the
// counter was incremented with failure_cause="dns".
func TestCheckConnectivityIncrementsFailureMetric(t *testing.T) {
	cfg := configmock.New(t)

	// Swap in a mock telemetry component so we can inspect counter increments.
	telemetryMock := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	metrics.TlmHTTPConnectivityFailure = telemetryMock.NewCounter(
		"logs", "http_connectivity_failure", []string{"failure_cause"}, "")

	// Point at an unresolvable host — guaranteed to fail with a DNS error.
	endpoint := config.NewEndpoint("test-api-key", "", "this-host-does-not-exist.invalid", 443, config.EmptyPathPrefix, true)

	result := CheckConnectivity(endpoint, cfg)
	assert.Equal(t, config.HTTPConnectivity(false), result, "expected connectivity check to fail")

	metric, err := telemetryMock.(telemetry.Mock).GetCountMetric("logs", "http_connectivity_failure")
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(metric), 1, "expected at least one failure_cause metric increment")
	assert.Equal(t, metrics.FailureCauseDNS, metric[0].Tags()["failure_cause"])
}

func destinationDropped(m *expvar.Map, host string) int64 {
	if v := m.Get(host); v != nil {
		return v.(*expvar.Int).Value()
	}
	return 0
}

func TestPendingDelegatedAuthDropsAfterBound(t *testing.T) {
	cfg := configmock.New(t)
	server := NewDelegatedAuthPendingTestServer(cfg)
	defer server.Stop()

	prevWait, prevInterval := delegatedAuthMaxWait, credentialWaitInterval
	delegatedAuthMaxWait = 50 * time.Millisecond
	credentialWaitInterval = 10 * time.Millisecond
	t.Cleanup(func() { delegatedAuthMaxWait, credentialWaitInterval = prevWait, prevInterval })

	droppedBefore := destinationDropped(&metrics.DestinationLogsDropped, server.Destination.host)
	delaDroppedBefore := destinationDropped(&metrics.DestinationDelegatedAuthDropped, server.Destination.host)

	payload := &message.Payload{MessageMetas: []*message.MessageMetadata{{}}, Encoded: []byte("test log")}
	output := make(chan *message.Payload, 1)
	isRetrying := make(chan bool, 1)
	done := make(chan destinationResult, 1)
	go func() { done <- server.Destination.sendAndRetry(payload, output, isRetrying) }()

	require.True(t, <-isRetrying, "the destination reports retrying while holding the payload")

	// Past the bound the hold ends as a drop without acking, with the dedicated metric.
	select {
	case result := <-done:
		assert.NoError(t, result.err)
	case <-time.After(5 * time.Second):
		t.Fatal("the delegated-auth hold did not end after the bound")
	}
	select {
	case <-output:
		t.Fatal("payload acked after the delegated-auth bound")
	default:
	}
	assert.Equal(t, int64(1), destinationDropped(&metrics.DestinationLogsDropped, server.Destination.host)-droppedBefore)
	assert.Equal(t, int64(1), destinationDropped(&metrics.DestinationDelegatedAuthDropped, server.Destination.host)-delaDroppedBefore)
}

func TestPendingDelegatedAuthLatchDropsImmediately(t *testing.T) {
	// After the bound fires once, later payloads drop without waiting a fresh bound.
	cfg := configmock.New(t)
	server := NewDelegatedAuthPendingTestServer(cfg)
	defer server.Stop()

	prevWait, prevInterval := delegatedAuthMaxWait, credentialWaitInterval
	delegatedAuthMaxWait = 2 * time.Second
	credentialWaitInterval = 10 * time.Millisecond
	t.Cleanup(func() { delegatedAuthMaxWait, credentialWaitInterval = prevWait, prevInterval })

	send := func() chan destinationResult {
		done := make(chan destinationResult, 1)
		payload := &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
		go func() { done <- server.Destination.sendAndRetry(payload, make(chan *message.Payload, 1), nil) }()
		return done
	}

	// First payload waits the full bound.
	select {
	case <-send():
	case <-time.After(5 * time.Second):
		t.Fatal("the first payload never ended its delegated-auth wait")
	}

	// The latch persists after the drop (structural, no timing).
	server.Destination.retryLock.Lock()
	latched := !server.Destination.pendingDelegatedAuthSince.IsZero()
	retrying := server.Destination.lastRetryError == nil
	server.Destination.retryLock.Unlock()
	assert.True(t, latched, "the pending latch must persist after the timeout drop")
	assert.True(t, retrying, "the timeout drop must clear the retry episode so the worker keeps feeding this destination")

	// The second payload must not wait a fresh bound: a fresh wait would take the full
	// 2s bound; 1s leaves a 2x margin on the discriminating dimension while tolerating
	// scheduling delay.
	select {
	case <-send():
	case <-time.After(1 * time.Second):
		t.Fatal("the latch did not drop the second payload immediately")
	}
}

func TestPendingDelegatedAuthRecoversAfterBound(t *testing.T) {
	// After the bound fired and dropped a payload, a key that arrives lets the
	// destination send again instead of dropping forever.
	cfg := configmock.New(t)
	server := NewTestServerWithOptions(http.StatusOK, 1, true, nil, cfg)
	defer server.Stop()

	prevWait, prevInterval := delegatedAuthMaxWait, credentialWaitInterval
	delegatedAuthMaxWait = 50 * time.Millisecond
	credentialWaitInterval = 10 * time.Millisecond
	t.Cleanup(func() { delegatedAuthMaxWait, credentialWaitInterval = prevWait, prevInterval })

	entry := func(apiKey string) []map[string]interface{} {
		return []map[string]interface{}{{
			"api_key": apiKey, "host": server.Endpoint.Host, "port": server.Endpoint.Port, "use_ssl": false,
		}}
	}
	cfg.SetInTest("logs_config.force_use_http", true)
	cfg.SetInTest("logs_config.additional_endpoints", entry("DELA[some-org-uuid, aws]"))
	endpoints, err := config.BuildHTTPEndpointsWithConfig(cfg, config.NewLogsConfigKeys("logs_config.", cfg), "agent-http-intake.logs.", "", "", "")
	require.NoError(t, err)
	require.Len(t, endpoints.Endpoints, 2)
	pending := endpoints.Endpoints[1]
	require.True(t, pending.AwaitsDelegatedAuthKey())
	server.Destination.endpoint = pending

	// First payload drops past the bound.
	payload := &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
	done := make(chan destinationResult, 1)
	go func() { done <- server.Destination.sendAndRetry(payload, make(chan *message.Payload, 1), nil) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the first payload never ended its delegated-auth wait")
	}

	// The write-back delivers the key: the next payload sends instead of dropping.
	cfg.Set("logs_config.additional_endpoints", entry("resolved-key"), pkgconfigmodel.SourceSecret)
	payload2 := &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("second log")}
	output2 := make(chan *message.Payload, 1)
	done2 := make(chan destinationResult, 1)
	go func() { done2 <- server.Destination.sendAndRetry(payload2, output2, nil) }()
	select {
	case sent := <-output2:
		assert.Same(t, payload2, sent)
	case <-time.After(5 * time.Second):
		t.Fatal("the destination did not resume sending after the key arrived")
	}
	assert.Equal(t, "resolved-key", server.request.Header.Get("DD-API-KEY"))
}

func TestPendingDelegatedAuthLatchStartsMidRetryEpisode(t *testing.T) {
	// A credential that turns pending while the destination is mid-retryable-episode
	// (lastRetryError set) must still start the latch clock; otherwise the hold is
	// unbounded again.
	cfg := configmock.New(t)
	server := NewDelegatedAuthPendingTestServer(cfg)
	defer server.Stop()

	prevWait, prevInterval := delegatedAuthMaxWait, credentialWaitInterval
	delegatedAuthMaxWait = 50 * time.Millisecond
	credentialWaitInterval = 10 * time.Millisecond
	t.Cleanup(func() { delegatedAuthMaxWait, credentialWaitInterval = prevWait, prevInterval })

	server.Destination.retryLock.Lock()
	server.Destination.lastRetryError = client.NewRetryableError(errors.New("server error"))
	server.Destination.nbErrors = 3 // a leftover send backoff must not slow the credential rechecks
	server.Destination.retryLock.Unlock()

	payload := &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("test log")}
	done := make(chan destinationResult, 1)
	go func() { done <- server.Destination.sendAndRetry(payload, make(chan *message.Payload, 1), nil) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the delegated-auth hold never ended: the latch did not start mid-retry-episode")
	}

	// The credential wait reset the send backoff so the recheck cadence is
	// credentialWaitInterval, not the backoff of the old episode.
	server.Destination.retryLock.Lock()
	nbErrors := server.Destination.nbErrors
	server.Destination.retryLock.Unlock()
	assert.Zero(t, nbErrors, "entering the delegated-auth wait must clear the leftover send backoff")
}

func TestDelegatedAuthLatchClearsOnRetryableSend(t *testing.T) {
	// A retryable send error implies the credential was ready: the pending clock must
	// reset so a later pending wait gets a fresh bound instead of dropping on a stale one.
	cfg := configmock.New(t)
	server := NewDelegatedAuthPendingTestServer(cfg)
	defer server.Stop()

	server.Destination.retryLock.Lock()
	server.Destination.pendingDelegatedAuthSince = time.Now().Add(-time.Hour)
	server.Destination.retryLock.Unlock()

	server.Destination.updateRetryState(client.NewRetryableError(errors.New("server error")), nil)

	server.Destination.retryLock.Lock()
	since := server.Destination.pendingDelegatedAuthSince
	seen := server.Destination.pendingDelegatedAuthTimeoutSeen
	server.Destination.retryLock.Unlock()
	assert.True(t, since.IsZero(), "a retryable send must clear the pending clock")
	assert.False(t, seen)
}
