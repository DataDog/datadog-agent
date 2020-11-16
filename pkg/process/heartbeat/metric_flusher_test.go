package heartbeat

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var then = time.Unix(123, 0)
var requestBody = `
{
  "series": [
    {
      "metric": "datadog.system_probe.agent.network_tracer",
      "points": [[123, 1]],
      "tags": ["version:8", "revision:123"],
      "host": "foobar",
      "type": "gauge",
      "interval": 0
    },
    {
      "metric": "datadog.system_probe.agent.oom_kill_probe",
      "points": [[123, 1]],
      "tags": ["version:8", "revision:123"],
      "host": "foobar",
      "type": "gauge",
      "interval": 0
    }
  ]
}
`

func TestFlush(t *testing.T) {
	mockForwarder := &forwarder.MockedForwarder{}
	tags := []string{"version:8", "revision:123"}
	flusher := &apiFlusher{
		forwarder:  mockForwarder,
		hostname:   "foobar",
		tags:       tags,
		apiWatcher: newAPIWatcher(time.Minute),
	}
	var payloads forwarder.Payloads

	mockForwarder.
		On("SubmitV1Series", mock.AnythingOfType("forwarder.Payloads"), mock.AnythingOfType("http.Header")).
		Return(nil).
		Times(1).
		Run(func(args mock.Arguments) {
			payloads = args.Get(0).(forwarder.Payloads)
		})

	flusher.Flush([]string{"datadog.system_probe.agent.network_tracer", "datadog.system_probe.agent.oom_kill_probe"}, then)
	mockForwarder.AssertExpectations(t)
	assert.JSONEq(t, requestBody, string(*payloads[0]))
}
