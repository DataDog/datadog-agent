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

func newServer(deps dependencies) (Component, error) {
	s, err := dogstatsd.NewServer(deps.Params.Serverless)
	if err != nil {
		return nil, err
	}
	return &server{server: s}, nil
}

func (s *server) Start(demultiplexer aggregator.Demultiplexer) {
	s.server.Start(demultiplexer)

}
func (s *server) Stop() {
	s.server.Stop()
}

func (s *server) Capture(p string, d time.Duration, compressed bool) error {
	return s.server.Capture(p, d, compressed)
}

func (s *server) IsCaputreOngoing() bool {
	return s.server.TCapture.IsOngoing()
}

func (s *server) GetCapturePath() (string, error) {
	return s.server.TCapture.Path()
}

func (s *server) GetJSONDebugStats() ([]byte, error) {
	return s.server.GetJSONDebugStats()
}

func (s *server) GetDebug() *dogstatsd.DsdServerDebug {
	return s.server.Debug
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
