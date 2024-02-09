// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

type serverMock struct {
	isRunning bool
}

func newMock() Component {
	return &serverMock{}
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) Start(demultiplexer aggregator.Demultiplexer) error {
	s.isRunning = true
	return nil
}

func (s *serverMock) Stop() {
	s.isRunning = false
}

func (s *serverMock) IsRunning() bool {
	return s.isRunning
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) Capture(p string, d time.Duration, compressed bool) (string, error) {
	return "", nil
}

func (s *serverMock) UdsListenerRunning() bool {
	return false
}

func (s *serverMock) UDPLocalAddr() string {
	return ""
}

func (s *serverMock) ServerlessFlush(time.Duration) {}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) SetExtraTags(tags []string) {}
