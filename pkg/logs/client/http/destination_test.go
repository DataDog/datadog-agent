// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package http

import (
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
}

func NewHTTPServerTest(statusCode int) *HTTPServerTest {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
	url := strings.Split(ts.URL, ":")
	port, _ := strconv.Atoi(url[2])
	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	dest := NewDestination(config.Endpoint{
		APIKey: "test",
		Host:   strings.Replace(url[1], "/", "", -1),
		Port:   port,
		UseSSL: false,
	}, destCtx)
	return &HTTPServerTest{
		httpServer:  ts,
		destCtx:     destCtx,
		destination: dest,
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
	assert.Equal(t, "https://foo/v1/input/bar", url)
}

func TestBuildURLShouldReturnHTTPWithoutUseSSL(t *testing.T) {
	url := buildURL(config.Endpoint{
		APIKey: "bar",
		Host:   "foo",
		UseSSL: false,
	})
	assert.Equal(t, "http://foo/v1/input/bar", url)
}

func TestBuildURLShouldReturnAddressWithPortWhenDefined(t *testing.T) {
	url := buildURL(config.Endpoint{
		APIKey: "bar",
		Host:   "foo",
		Port:   1234,
		UseSSL: false,
	})
	assert.Equal(t, "http://foo:1234/v1/input/bar", url)
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
	assert.Equal(t, "server error", err.Error())
	server.stop()
}

func TestDestinationSend400(t *testing.T) {
	server := NewHTTPServerTest(400)
	err := server.destination.Send([]byte("yo"))
	assert.NotNil(t, err)
	assert.Equal(t, "client error", err.Error())
	server.stop()
}
