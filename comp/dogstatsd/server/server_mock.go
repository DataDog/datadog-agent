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
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) Start(demultiplexer aggregator.Demultiplexer) error {
	panic("not called")
}

func (s *serverMock) Stop() {
	panic("not called")
}

func (s *serverMock) IsRunning() bool {
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) Capture(p string, d time.Duration, compressed bool) (string, error) {
	panic("not called")
}

func (s *serverMock) UdsListenerRunning() bool {
	panic("not called")
}

func (s *serverMock) UDPLocalAddr() string {
	panic("not called")
}

func (s *serverMock) ServerlessFlush(time.Duration) {
	panic("not called")
}

//nolint:revive // TODO(AML) Fix revive linter
func (s *serverMock) SetExtraTags(tags []string) {
	panic("not called")
}
