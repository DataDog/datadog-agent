package heartbeat

import (
	"fmt"
	"time"

	pstatsd "github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-go/statsd"
)

type statsdFlusher struct {
	client statsd.ClientInterface
	tags   []string
}

var _ flusher = &statsdFlusher{}

func newStatsdFlusher(opts Options) (flusher, error) {
	client := opts.StatsdClient
	if client == nil {
		client = pstatsd.Client
	}
	if client == nil {
		return nil, fmt.Errorf("missing statsd client")
	}

	return &statsdFlusher{
		client: client,
		tags:   tagsFromOptions(opts),
	}, nil
}

// Flush heartbeats via statsd
func (f *statsdFlusher) Flush(metricNames []string, _ time.Time) {
	for _, name := range metricNames {
		f.client.Gauge(name, 1, f.tags, 1) //nolint:errcheck
	}
}

// Stop flusher
func (f *statsdFlusher) Stop() {}
