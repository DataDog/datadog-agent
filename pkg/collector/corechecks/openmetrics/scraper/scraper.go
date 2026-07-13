// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

const (
	serviceCheckHealth = "openmetrics.health"
)

// Scraper fetches, parses, transforms, and submits metrics from an OpenMetrics
// or Prometheus endpoint. It is designed to be embedded by Go core checks.
type Scraper struct {
	config          *Config
	httpClient      *http.Client
	filter          *MetricFilter
	transformer     *MetricTransformer
	labelProcessor  *LabelProcessor
	labelAggregator *LabelAggregator

	endpoint        string
	namespace       string
	rawMetricPrefix string
	flushFirstValue bool
	tagByEndpoint   bool
	endpointTag     string

	enableHealthServiceCheck bool
	ignoreConnectionErrors   bool
	maxReturnedMetrics       int
}

// NewScraper creates a Scraper from a resolved and validated Config.
func NewScraper(cfg *Config) (*Scraper, error) {
	filter, err := NewMetricFilter(
		cfg.Metrics,
		cfg.ExtraMetrics,
		cfg.ExcludeMetrics,
		cfg.ExcludeMetricsByLabels,
		boolVal(cfg.CacheMetricWildcards, true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build metric filter: %w", err)
	}

	transformer := NewMetricTransformer(cfg, filter)
	labelProcessor := NewLabelProcessor(cfg)

	var labelAggregator *LabelAggregator
	if len(cfg.ShareLabels) > 0 {
		labelAggregator = NewLabelAggregator(cfg.ShareLabels, boolVal(cfg.CacheSharedLabels, true))
	}

	s := &Scraper{
		config:          cfg,
		httpClient:      buildHTTPClient(cfg),
		filter:          filter,
		transformer:     transformer,
		labelProcessor:  labelProcessor,
		labelAggregator: labelAggregator,

		endpoint:        cfg.Endpoint(),
		namespace:       strings.TrimSuffix(cfg.Namespace, "."),
		rawMetricPrefix: cfg.RawMetricPrefix,

		enableHealthServiceCheck: boolVal(cfg.EnableHealthServiceCheck, true),
		ignoreConnectionErrors:   boolVal(cfg.IgnoreConnectionErrors, false),
		maxReturnedMetrics:       cfg.MaxReturnedMetrics,
		tagByEndpoint:            boolVal(cfg.TagByEndpoint, false),
	}

	if s.tagByEndpoint {
		s.endpointTag = "endpoint:" + s.endpoint
	}

	return s, nil
}

// Scrape executes a single collection cycle: fetch the endpoint, parse metrics,
// apply filters and transforms, and submit them via the Sender.
func (s *Scraper) Scrape(snd sender.Sender) error {
	body, contentType, err := s.fetchEndpoint()
	if err != nil {
		if s.enableHealthServiceCheck {
			snd.ServiceCheck(serviceCheckHealth, servicecheck.ServiceCheckCritical, "", nil, err.Error())
		}
		if s.ignoreConnectionErrors {
			log.Debugf("openmetrics: ignoring connection error for %s: %v", s.endpoint, err)
			return nil
		}
		return err
	}

	if s.enableHealthServiceCheck {
		snd.ServiceCheck(serviceCheckHealth, servicecheck.ServiceCheckOK, "", nil, "")
	}

	families, err := prometheus.ParseMetricsFromResponse(body, contentType, s.config.RawLineFilters)
	if err != nil {
		return fmt.Errorf("openmetrics: failed to parse metrics from %s: %w", s.endpoint, err)
	}

	// Run label aggregator collection pass.
	if s.labelAggregator != nil {
		s.labelAggregator.Process(families)
	}

	metricsSubmitted := 0

	for i := range families {
		family := &families[i]

		// Check max_returned_metrics limit.
		if s.maxReturnedMetrics > 0 && metricsSubmitted >= s.maxReturnedMetrics {
			log.Debugf("openmetrics: reached max_returned_metrics limit (%d)", s.maxReturnedMetrics)
			break
		}

		rawName := family.Name

		// Strip raw_metric_prefix if configured.
		if s.rawMetricPrefix != "" {
			rawName = strings.TrimPrefix(rawName, s.rawMetricPrefix)
		}

		// Check metric filter.
		match, ok := s.filter.MatchMetric(rawName)
		if !ok {
			continue
		}

		// Determine the Datadog metric name.
		ddName := match.Name
		if ddName == "" {
			ddName = rawName
		}

		// Apply namespace prefix.
		ddName = s.metricNameWithNamespace(ddName)

		// Resolve the transformer.
		var tf TransformerFunc
		if match.Type != "" && match.Type != defaultMetricType {
			// The filter returned an explicit type override.
			tf = s.transformer.Get(rawName, match.Type)
		} else {
			// Use the endpoint-reported type.
			tf = s.transformer.Get(rawName, family.Type)
		}
		if tf == nil {
			continue
		}

		// Build SampleData for all non-excluded, non-NaN/Inf samples.
		samples := s.buildSampleData(family)
		if len(samples) == 0 {
			continue
		}

		// Submit.
		tf(ddName, samples, snd, s.flushFirstValue)
		metricsSubmitted++
	}

	// After the first successful scrape, flush first values for monotonic counters.
	s.flushFirstValue = true

	return nil
}

// buildSampleData processes a MetricFamily's samples through label processing,
// label aggregation, and sample-level filtering.
func (s *Scraper) buildSampleData(family *prometheus.MetricFamily) []SampleData {
	result := make([]SampleData, 0, len(family.Samples))

	for i := range family.Samples {
		sample := &family.Samples[i]

		// Skip NaN/Inf values.
		if math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) {
			continue
		}

		// Check exclude by labels.
		if s.filter.ShouldExcludeSample(sample.Metric) {
			continue
		}

		// Build tags from labels.
		tags, hostname := s.labelProcessor.ProcessLabels(sample.Metric, family.Type)

		// Append shared labels from label aggregator.
		if s.labelAggregator != nil {
			shared := s.labelAggregator.GetSharedTags(sample.Metric)
			if len(shared) > 0 {
				tags = append(tags, shared...)
			}
		}

		// Append endpoint tag if configured.
		if s.tagByEndpoint {
			tags = append(tags, s.endpointTag)
		}

		result = append(result, SampleData{
			Sample:   sample,
			Tags:     tags,
			Hostname: hostname,
		})
	}

	return result
}

// fetchEndpoint executes an HTTP GET against the configured endpoint.
func (s *Scraper) fetchEndpoint() (body []byte, contentType string, err error) {
	req, err := http.NewRequest(http.MethodGet, s.endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set Accept header to prefer OpenMetrics format.
	req.Header.Set("Accept", "application/openmetrics-text;version=1.0.0,application/openmetrics-text;version=0.0.1;q=0.75,text/plain;version=0.0.4;q=0.5,*/*;q=0.1")

	// Apply configured headers.
	for k, v := range s.config.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range s.config.ExtraHeaders {
		req.Header.Set(k, v)
	}

	// Basic auth.
	if s.config.Username != "" {
		req.SetBasicAuth(s.config.Username, s.config.Password)
	}

	// Bearer token.
	if s.config.BearerTokenAuth {
		token, err := s.readBearerToken()
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("request to %s failed: %w", s.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, s.endpoint)
	}

	contentType = resp.Header.Get("Content-Type")

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body from %s: %w", s.endpoint, err)
	}

	return body, contentType, nil
}

// readBearerToken reads the bearer token from the configured path.
func (s *Scraper) readBearerToken() (string, error) {
	path := s.config.BearerTokenPath
	if path == "" {
		path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read bearer token from %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// metricNameWithNamespace prepends the configured namespace to a metric name.
func (s *Scraper) metricNameWithNamespace(name string) string {
	if s.namespace == "" {
		return name
	}
	return s.namespace + "." + name
}

// buildHTTPClient creates an *http.Client from the scraper Config.
func buildHTTPClient(cfg *Config) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   !boolVal(cfg.PersistConnections, true),
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// TLS configuration.
	if cfg.TLSCert != "" || cfg.TLSCACert != "" || (cfg.TLSVerify != nil && !*cfg.TLSVerify) {
		tlsCfg := &tls.Config{}
		if cfg.TLSVerify != nil {
			tlsCfg.InsecureSkipVerify = !*cfg.TLSVerify
		}
		transport.TLSClientConfig = tlsCfg
	}

	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(cfg.Timeout) * time.Second,
	}
}
