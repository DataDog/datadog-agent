// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

type HTTPServerTest struct {
	httpServer  *httptest.Server
	destCtx     *client.DestinationsContext
	destination *Destination
	endpoint    config.Endpoint
	request     *http.Request
}

func NewHTTPServerTest(statusCode int) *HTTPServerTest {
	var request http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		request = *r
	}))
	url := strings.Split(ts.URL, ":")
	port, _ := strconv.Atoi(url[2])
	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	endpoint := config.Endpoint{
		APIKey: "test",
		Host:   strings.Replace(url[1], "/", "", -1),
		Port:   port,
		UseSSL: false,
	}
	dest := NewDestination(endpoint, JSONContentType, destCtx, 0)
	return &HTTPServerTest{
		httpServer:  ts,
		destCtx:     destCtx,
		destination: dest,
		endpoint:    endpoint,
		request:     &request,
	}
}

func (s *HTTPServerTest) stop() {
	s.destCtx.Start()
	s.httpServer.Close()
}

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
	server := NewHTTPServerTest(200)
	err := server.destination.Send([]byte("yo"))
	assert.Nil(t, err)
	server.stop()
}

func TestDestinationSend500(t *testing.T) {
	server := NewHTTPServerTest(500)
	err := server.destination.Send([]byte("yo"))
	assert.NotNil(t, err)
	_, retriable := err.(*client.RetryableError)
	assert.True(t, retriable)
	assert.Equal(t, "server error", err.Error())
	server.stop()
}

func TestDestinationSend429(t *testing.T) {
	server := NewHTTPServerTest(429)
	err := server.destination.Send([]byte("yo"))
	assert.NotNil(t, err)
	_, retriable := err.(*client.RetryableError)
	assert.True(t, retriable)
	assert.Equal(t, "server error", err.Error())
	server.stop()
}

func TestDestinationSend400(t *testing.T) {
	server := NewHTTPServerTest(400)
	err := server.destination.Send([]byte("yo"))
	assert.NotNil(t, err)
	_, retriable := err.(*client.RetryableError)
	assert.False(t, retriable)
	assert.Equal(t, "client error", err.Error())
	server.stop()
}

func TestConnectivityCheck(t *testing.T) {
	// Connectivity is ok when server return 200
	server := NewHTTPServerTest(200)
	connectivity := CheckConnectivity(server.endpoint)
	assert.Equal(t, config.HTTPConnectivitySuccess, connectivity)
	server.stop()

	// Connectivity is ok when server return 500
	server = NewHTTPServerTest(500)
	connectivity = CheckConnectivity(server.endpoint)
	assert.Equal(t, config.HTTPConnectivityFailure, connectivity)
	server.stop()
}

func TestErrorToTag(t *testing.T) {
	assert.Equal(t, errorToTag(nil), "none")
	assert.Equal(t, errorToTag(errors.New("fail")), "non-retryable")
	assert.Equal(t, errorToTag(client.NewRetryableError(errors.New("fail"))), "retryable")
}

func TestDestinationSendsV2Protocol(t *testing.T) {
	server := NewHTTPServerTest(200)
	defer server.httpServer.Close()

	server.destination.protocol = "test-proto"
	err := server.destination.unconditionalSend([]byte("payload"))
	assert.Nil(t, err)
	assert.Equal(t, server.request.Header.Get("dd-protocol"), "test-proto")
}

func TestDestinationDoesntSendEmptyV2Protocol(t *testing.T) {
	server := NewHTTPServerTest(200)
	defer server.httpServer.Close()

	err := server.destination.unconditionalSend([]byte("payload"))
	assert.Nil(t, err)
	assert.Empty(t, server.request.Header.Values("dd-protocol"))
}
