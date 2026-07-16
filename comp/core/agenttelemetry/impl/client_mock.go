// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package agenttelemetryimpl

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// ClientMock is a mock implementation of Client. It records every request
// and body Do receives -- not just the last one -- so tests can assert on
// batch/multi-POST behavior, and returns a configurable status code or, if
// Err is set, an error instead of a response.
type ClientMock struct {
	mu sync.Mutex

	requests []*http.Request
	bodies   [][]byte

	// StatusCode is the HTTP status Do returns. Defaults to 200 if zero.
	StatusCode int
	// Err, if set, is returned by Do instead of a response.
	Err error
}

// NewClientMock returns a Client that records every request it receives
// and responds with 200 OK.
func NewClientMock() *ClientMock {
	return &ClientMock{}
}

// Do implements Client.
func (c *ClientMock) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.requests = append(c.requests, req)
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		c.bodies = append(c.bodies, body)
	} else {
		c.bodies = append(c.bodies, nil)
	}

	if c.Err != nil {
		return nil, c.Err
	}
	status := c.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

// Requests returns a snapshot of every request Do has received, in call order.
func (c *ClientMock) Requests() []*http.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*http.Request{}, c.requests...)
}

// Bodies returns a snapshot of every request body Do has received, in call
// order. A nil entry means that call's request had no body.
func (c *ClientMock) Bodies() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	bodies := make([][]byte, 0, len(c.bodies))
	for _, b := range c.bodies {
		if b == nil {
			bodies = append(bodies, nil)
			continue
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		bodies = append(bodies, cp)
	}
	return bodies
}

// Body returns the body of the most recent request Do has received, or nil
// if Do has not been called yet.
func (c *ClientMock) Body() []byte {
	bodies := c.Bodies()
	if len(bodies) == 0 {
		return nil
	}
	return bodies[len(bodies)-1]
}
