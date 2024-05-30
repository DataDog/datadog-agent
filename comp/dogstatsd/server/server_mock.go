// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"net/http"
	"time"

	"go.uber.org/fx"

	apihelper "github.com/DataDog/datadog-agent/comp/api/api/helpers"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type serverMock struct {
	isRunning bool
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp     Component
	Endpoint apihelper.AgentEndpointProvider
}

func newMock() MockProvides {
	m := &serverMock{}
	return MockProvides{
		Comp:     m,
		Endpoint: apihelper.NewAgentEndpointProvider(m.handlerFunc, "/dogstatsd-stats", "GET"),
	}
}

// ServeHTTP is a simple mocked http.Handler function
func (s *serverMock) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) Start(demultiplexer aggregator.Demultiplexer) error {
	s.isRunning = true
	return nil
}

// Stop is a mocked function that flips isRunning to false
func (s *serverMock) Stop() {
	s.isRunning = false
}

// IsRunning is a mocked function that returns whether the mock was set to running
func (s *serverMock) IsRunning() bool {
	return s.isRunning
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) Capture(p string, d time.Duration, compressed bool) (string, error) {
	return "", nil
}

// UdsListenerRunning is a mocked function that returns false
func (s *serverMock) UdsListenerRunning() bool {
	return false
}

// UDPLocalAddr is a mocked function but UDP isn't enabled on the mock
func (s *serverMock) UDPLocalAddr() string {
	return ""
}

// ServerlessFlush is a noop mocked function
func (s *serverMock) ServerlessFlush(time.Duration) {}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) SetExtraTags(tags []string) {}
