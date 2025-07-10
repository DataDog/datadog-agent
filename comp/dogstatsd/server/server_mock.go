// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"time"

	"go.uber.org/fx"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type serverMock struct {
	isRunning bool
	blocklist []string
}

// MockProvides is the mock component output
type MockProvides struct {
	fx.Out

	Comp     Component
	Endpoint api.AgentEndpointProvider
}

func newMock() MockProvides {
	m := &serverMock{}
	return MockProvides{
		Comp: m,
	}
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) Start(_ aggregator.Demultiplexer) error {
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
func (s *serverMock) Capture(_ string, _ time.Duration, _ bool) (string, error) {
	return "", nil
}

// UDPLocalAddr is a mocked function but UDP isn't enabled on the mock
func (s *serverMock) UDPLocalAddr() string {
	return ""
}

func (s *serverMock) SetBlocklist(v []string, _ bool) {
	s.blocklist = v
}

// ServerlessFlush is a noop mocked function
func (s *serverMock) ServerlessFlush(time.Duration) {}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) SetExtraTags(_ []string) {}
