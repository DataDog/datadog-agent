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

func newServer(deps dependencies) Component {
	return &server{server: dogstatsd.NewServer(deps.Params.Serverless)}
}

func (s *server) Start(demultiplexer aggregator.Demultiplexer) error {
	return s.server.Start(demultiplexer)

}
func (s *server) Stop() {
	s.server.Stop()
}

func (s *server) Capture(p string, d time.Duration, compressed bool) (string, error) {

	err := s.server.Capture(p, d, compressed)
	if err != nil {
		return "", err
	}

	// wait for the capture to start
	for !s.server.TCapture.IsOngoing() {
		time.Sleep(500 * time.Millisecond)
	}

	path, err := s.server.TCapture.Path()

	return path, err
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
