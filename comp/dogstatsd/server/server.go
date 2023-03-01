// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Log    log.Component
	Params Params
}

type server struct {
	server *dogstatsd.Server
}

// TODO: (components) - remove once serverless is an FX app
func NewServerlessServer() Component {
	return &server{server: dogstatsd.NewServer(true)}
}

func newServer(deps dependencies) Component {
	return &server{server: dogstatsd.NewServer(deps.Params.Serverless)}
}

func (s *server) Start(demultiplexer aggregator.Demultiplexer) error {
	return s.server.Start(demultiplexer)

}
func (s *server) Stop() {
	s.server.Stop()
}

func (s *server) IsRunning() bool {
	return s.server.Started
}

func (s *server) Capture(p string, d time.Duration, compressed bool) (string, error) {
	return s.server.Capture(p, d, compressed)
}

func (s *server) GetJSONDebugStats() ([]byte, error) {
	return s.server.GetJSONDebugStats()
}

func (s *server) IsDebugEnabled() bool {
	return s.server.Debug.Enabled.Load()
}

func (s *server) EnableMetricsStats() {
	s.server.EnableMetricsStats()
}

func (s *server) DisableMetricsStats() {
	s.server.DisableMetricsStats()
}

func (s *server) UdsListenerRunning() bool {
	return s.server.UdsListenerRunning
}

func (s *server) ServerlessFlush() {
	s.server.ServerlessFlush()
}

func (s *server) SetExtraTags(tags []string) {
	s.server.SetExtraTags(tags)
}
