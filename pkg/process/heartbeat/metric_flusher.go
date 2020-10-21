package heartbeat

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type flusher interface {
	Flush(metricNames []string, now time.Time)
	Stop()
}

type apiFlusher struct {
	forwarder  forwarder.Forwarder
	apiWatcher *apiWatcher
	fallback   flusher
	tags       []string
	hostname   string
}

var _ flusher = &apiFlusher{}

func newAPIFlusher(opts Options, fallback flusher) (flusher, error) {
	if len(opts.KeysPerDomain) == 0 {
		return nil, fmt.Errorf("missing api key information")
	}
	if len(opts.HostName) == 0 {
		return nil, fmt.Errorf("missing hostname information")
	}

	apiWatcher := newAPIWatcher(time.Minute)

	// Instantiate forwarder responsible for sending hearbeat metrics to the API
	fwdOpts := forwarder.NewOptions(sanitize(opts.KeysPerDomain))
	fwdOpts.DisableAPIKeyChecking = true
	fwdOpts.CompletionHandler = apiWatcher.handler()
	heartbeatForwarder := forwarder.NewDefaultForwarder(fwdOpts)
	err := heartbeatForwarder.Start()
	if err != nil {
		return nil, err
	}

	return &apiFlusher{
		forwarder:  heartbeatForwarder,
		apiWatcher: apiWatcher,
		fallback:   fallback,
		tags:       tagsFromOptions(opts),
		hostname:   opts.HostName,
	}, nil
}

// Flush heartbeats metrics via the API
func (f *apiFlusher) Flush(metricNames []string, now time.Time) {
	if len(metricNames) == 0 {
		return
	}

	if f.apiWatcher.state() == apiUnreachable && f.fallback != nil {
		f.fallback.Flush(metricNames, now)
		return
	}

	heartbeats, err := f.jsonPayload(metricNames, now)
	if err != nil {
		log.Errorf("error marshaling heartbeats payload: %s", err)
		return
	}

	payload := forwarder.Payloads{&heartbeats}
	f.forwarder.SubmitV1Series(payload, nil) //nolint:errcheck
}

// Stop forwarder
func (f *apiFlusher) Stop() {
	f.forwarder.Stop()
}

func (f *apiFlusher) jsonPayload(metricNames []string, now time.Time) ([]byte, error) {
	if len(metricNames) == 0 {
		return nil, nil
	}

	ts := float64(now.Unix())
	heartbeats := make(metrics.Series, 0, len(metricNames))
	for _, name := range metricNames {
		serie := &metrics.Serie{
			Name: name,
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

func sanitize(keysPerDomain map[string][]string) map[string][]string {
	sanitized := make(map[string][]string)
	for domain, keys := range keysPerDomain {
		sanitizedDomain := strings.Replace(domain, "process", "app", 1)
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
		tags = append(tags, fmt.Sprintf("revision:%s", opts.TagRevision))
	}
	return tags
}
