package writer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestTraceWriter(t *testing.T) {
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   "agent-unit",
		DefaultEnv: "testing",
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
	}
	t.Run("ok", func(t *testing.T) {
		testTrace := &SampledSpans{Trace: testutil.RandomTrace(4, 100)}
		defer useFlushThreshold(testTrace.size()*2 + 10)()
		in := make(chan *SampledSpans, 100)
		tw := NewTraceWriter(cfg, in)
		go tw.Run()
		in <- testTrace
		in <- testTrace
		in <- testTrace
		tw.Stop()
		assert.Equal(t, 2, srv.Accepted())
	})
}

func useFlushThreshold(n int) func() {
	old := payloadFlushThreshold
	payloadFlushThreshold = n
	return func() { payloadFlushThreshold = old }
}
