package heartbeat

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pstatsd "github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/statsd"
)

const metricName = "datadog.system_probe.agent.%s"

type flusher interface {
	Flush(modules []string, now time.Time)
	Stop()
}

type apiFlusher struct {
	forwarder forwarder.Forwarder
	fallback  flusher
	tags      []string
	hostname  string
}

var _ flusher = &apiFlusher{}

func newAPIFlusher(opts Options, fallback flusher) (flusher, error) {
	if len(opts.KeysPerDomain) == 0 {
		return nil, fmt.Errorf("missing api key information")
	}
	if len(opts.HostName) == 0 {
		return nil, fmt.Errorf("missing hostname information")
	}

	// Instantiate forwarder responsible for sending hearbeat metrics to the API
	fwdOpts := forwarder.NewOptions(sanitize(opts.KeysPerDomain))
	fwdOpts.DisableAPIKeyChecking = true
	heartbeatForwarder := forwarder.NewDefaultForwarder(fwdOpts)
	heartbeatForwarder.Start()

	return &apiFlusher{
		forwarder: heartbeatForwarder,
		fallback:  fallback,
		tags:      tagsFromOptions(opts),
		hostname:  opts.HostName,
	}, nil
}

// Flush heartbeats metrics for each system-probe module to Datadog.  We first
// attempt to flush it via the Metrics API. In case of failures we fallback to
// `statsd`.
func (f *apiFlusher) Flush(modules []string, now time.Time) {
	if len(modules) == 0 {
		return
	}

	heartbeats, err := f.jsonPayload(modules, now)
	if err != nil {
		log.Errorf("error marshaling heartbeats payload: %s", err)
		return
	}

	payload := forwarder.Payloads{&heartbeats}
	if err := f.forwarder.SubmitV1Series(payload, http.Header{}); err != nil && f.fallback != nil {
		log.Errorf("could not flush heartbeats to API: %s. trying statsd...", err)
		f.fallback.Flush(modules, now)
	}
}

// Stop forwarder
func (f *apiFlusher) Stop() {
	f.forwarder.Stop()
}

func (f *apiFlusher) jsonPayload(modules []string, now time.Time) ([]byte, error) {
	if len(modules) == 0 {
		return nil, nil
	}

	ts := float64(now.Unix())
	heartbeats := make(metrics.Series, 0, len(modules))
	for _, moduleName := range modules {
		serie := &metrics.Serie{
			Name: fmt.Sprintf(metricName, moduleName),
			Tags: f.tags,
			Host: f.hostname,
			Points: []metrics.Point{
				{
					Ts:    ts,
					Value: float64(1),
				},
			},
		}
		heartbeats = append(heartbeats, serie)
	}

	return heartbeats.MarshalJSON()
}

type statsdFlusher struct {
	client statsd.ClientInterface
	tags   []string
}

var _ flusher = &statsdFlusher{}

func newStatsdFlusher(opts Options) (flusher, error) {
	if opts.StatsdClient != nil {
		opts.StatsdClient = pstatsd.Client
	}
	if opts.StatsdClient != nil {
		return nil, fmt.Errorf("missing statsd client")
	}

	return &statsdFlusher{
		client: opts.StatsdClient,
		tags:   tagsFromOptions(opts),
	}, nil
}

// Flush heartbeats via statsd
func (f *statsdFlusher) Flush(modules []string, _ time.Time) {
	for _, moduleName := range modules {
		f.client.Gauge(fmt.Sprintf(metricName, moduleName), 1, f.tags, 1) //nolint:errcheck
	}
}

// Stop flusher
func (f *statsdFlusher) Stop() {}

func sanitize(keysPerDomain map[string][]string) map[string][]string {
	sanitized := make(map[string][]string)
	for domain, keys := range keysPerDomain {
		sanitizedDomain := strings.ReplaceAll(domain, "process", "app")
		sanitized[sanitizedDomain] = keys
	}
	return sanitized
}

func tagsFromOptions(opts Options) []string {
	var tags []string
	if opts.TagVersion != "" {
		tags = append(tags, fmt.Sprintf("version:%s", opts.TagVersion))
	}
	if opts.TagRevision != "" {
		tags = append(tags, fmt.Sprintf("version:%s", opts.TagRevision))
	}
	return tags
}
