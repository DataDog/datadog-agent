// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package csidriver implements the CSI driver core check.
package csidriver

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	prometheus "github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// CheckName is the name of the check as registered in the loader.
const CheckName = "datadog_csi_driver"

const (
	metricNs          = "datadog.csi_driver."
	httpClientTimeout = 5 * time.Second
)

type metricKind int

const (
	counterMetric metricKind = iota
	gaugeMetric
	histogramMetric
)

// coatMetric associates a COAT telemetry metric with the Prometheus label names
// it expects, in the order required by the telemetry component.
type coatMetric struct {
	counter telemetry.Counter
	gauge   telemetry.Gauge
	kind    metricKind
	tagKeys []string
}

// coatHistogram mirrors a Prometheus histogram into COAT by reducing it to its
// _count and _sum series, exposed as two counters. We intentionally drop the
// per-bucket distribution (COAT keeps volume and average latency, not
// percentiles): the telemetry.Histogram API only offers Observe(), which cannot
// faithfully ingest an already-cumulative external histogram, whereas _count and
// _sum are exact cumulative counters that mirror cleanly and aggregate correctly.
type coatHistogram struct {
	count   telemetry.Counter
	sum     telemetry.Counter
	tagKeys []string
}

// metricDef defines how a Prometheus metric is mapped to a Datadog metric name
// and optionally to COAT telemetry (a counter/gauge, or a histogram reduced to
// count/sum counters).
type metricDef struct {
	ddName   string
	kind     metricKind
	coat     *coatMetric
	coatHist *coatHistogram
}

// sharedState is COAT telemetry bookkeeping shared across every Check instance
// from the same Factory, because the telemetry registry is process-global and
// outlives a check (autodiscovery may recreate it on CSI driver pod restart).
//
// Single-endpoint assumption: the CSI driver is a per-node DaemonSet scraped
// over localhost (conf.d/datadog_csi_driver.d/auto_conf.yaml), so an Agent only
// ever runs one datadog_csi_driver instance. We therefore do NOT aggregate gauge
// values across endpoints: for a given COAT series (preserved tags, e.g.
// "library") the latest scrape wins. Counters are keyed per endpoint since
// summing per-scrape deltas is correct regardless of instance count.
type sharedState struct {
	prevValues map[string]float64     // counter delta cache, keyed per endpoint+preserved-tag context
	gaugeKeys  map[string]gaugeSeries // gauge series from the last successful scrape, keyed by COAT series identity
	mu         sync.Mutex
}

// gaugeSeries remembers a gauge series so it can be deleted from the global
// telemetry registry once it disappears from the scrape (or on cancel/failure),
// avoiding stale values being reported until Agent restart.
type gaugeSeries struct {
	coat      *coatMetric
	tagValues []string
}

func buildMetricDefs(tm telemetry.Component) map[string]metricDef {
	return map[string]metricDef{
		"datadog_csi_driver_node_publish_volume_attempts": {
			ddName: "node_publish_volume_attempts",
			kind:   counterMetric,
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "node_publish_volume_attempts", []string{"status", "type"}, "CSI node publish volume attempts"),
				kind:    counterMetric,
				tagKeys: []string{"status", "type"},
			},
		},
		"datadog_csi_driver_node_unpublish_volume_attempts": {
			ddName: "node_unpublish_volume_attempts",
			kind:   counterMetric,
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "node_unpublish_volume_attempts", []string{"status"}, "CSI node unpublish volume attempts"),
				kind:    counterMetric,
				tagKeys: []string{"status"},
			},
		},
		"datadog_csi_driver_library_resolutions": {
			ddName: "library_resolutions",
			kind:   counterMetric,
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "library_resolutions", []string{"library", "result"}, "CSI driver library resolution attempts"),
				kind:    counterMetric,
				tagKeys: []string{"library", "result"},
			},
		},
		"datadog_csi_driver_library_download_duration_seconds": {
			ddName: "library_download_duration_seconds",
			kind:   histogramMetric,
			coatHist: &coatHistogram{
				count:   tm.NewCounter(CheckName, "library_download_duration_seconds_count", []string{"library", "registry"}, "CSI driver library downloads"),
				sum:     tm.NewCounter(CheckName, "library_download_duration_seconds_sum", []string{"library", "registry"}, "CSI driver library download duration total seconds"),
				tagKeys: []string{"library", "registry"},
			},
		},
		"datadog_csi_driver_library_cleanup": {
			ddName: "library_cleanup",
			kind:   counterMetric,
			coat: &coatMetric{
				counter: tm.NewCounter(CheckName, "library_cleanup", []string{"library", "status", "strategy"}, "CSI driver library cleanup attempts"),
				kind:    counterMetric,
				tagKeys: []string{"library", "status", "strategy"},
			},
		},
		"datadog_csi_driver_libraries_cached": {
			ddName: "libraries_cached",
			kind:   gaugeMetric,
			coat: &coatMetric{
				gauge:   tm.NewGauge(CheckName, "libraries_cached", []string{"library"}, "CSI driver cached library versions"),
				kind:    gaugeMetric,
				tagKeys: []string{"library"},
			},
		},
		"datadog_csi_driver_libraries_cached_bytes": {
			ddName: "libraries_cached_bytes",
			kind:   gaugeMetric,
			coat: &coatMetric{
				gauge:   tm.NewGauge(CheckName, "libraries_cached_bytes", []string{"library"}, "CSI driver cached library bytes"),
				kind:    gaugeMetric,
				tagKeys: []string{"library"},
			},
		},
		"datadog_csi_driver_library_volume_links": {
			ddName: "library_volume_links",
			kind:   gaugeMetric,
			coat: &coatMetric{
				gauge:   tm.NewGauge(CheckName, "library_volume_links", []string{"library"}, "CSI driver library volume links"),
				kind:    gaugeMetric,
				tagKeys: []string{"library"},
			},
		},
	}
}

// Check collects CSI driver metrics from its Prometheus endpoint.
type Check struct {
	core.CheckBase
	config     csiDriverConfig
	httpClient http.Client
	metrics    map[string]metricDef
	state      *sharedState
}

// Factory creates a new check factory.
func Factory(tm telemetry.Component) option.Option[func() check.Check] {
	metricDefs := buildMetricDefs(tm)
	state := &sharedState{
		prevValues: make(map[string]float64),
		gaugeKeys:  make(map[string]gaugeSeries),
	}
	return option.New(func() check.Check {
		return &Check{
			CheckBase: core.NewCheckBase(CheckName),
			metrics:   metricDefs,
			state:     state,
		}
	})
}

// Configure parses the check configuration and initialises the HTTP client.
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return fmt.Errorf("configure %s: %w", CheckName, err)
	}

	if err := c.config.parse(config); err != nil {
		return err
	}

	c.httpClient = http.Client{Timeout: httpClientTimeout}

	log.Debugf("%s: configured (endpoint: %s)", CheckName, c.config.OpenmetricsEndpoint)

	return nil
}

// Run scrapes the CSI driver metrics endpoint and submits metrics.
func (c *Check) Run() error {
	s, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get sender: %w", err)
	}
	defer s.Commit()

	metrics, err := c.scrape()
	if err != nil {
		c.deleteAllGaugeSeries()
		s.ServiceCheck(metricNs+"openmetrics.health", servicecheck.ServiceCheckCritical, "", nil, fmt.Sprintf("endpoint unreachable: %s", err))
		return fmt.Errorf("scrape %s: %w", c.config.OpenmetricsEndpoint, err)
	}

	s.ServiceCheck(metricNs+"openmetrics.health", servicecheck.ServiceCheckOK, "", nil, "")

	c.submitMetrics(s, metrics)

	return nil
}

// Cancel clears long-lived COAT gauge series when the check is unscheduled.
func (c *Check) Cancel() {
	c.deleteAllGaugeSeries()
	c.CheckBase.Cancel()
}

func (c *Check) scrape() ([]prometheus.MetricFamily, error) {
	resp, err := c.httpClient.Get(c.config.OpenmetricsEndpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d from %s", resp.StatusCode, c.config.OpenmetricsEndpoint)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	families, err := prometheus.ParseMetrics(body)
	if err != nil {
		return nil, fmt.Errorf("parse prometheus metrics: %w", err)
	}

	return families, nil
}

// normalizeMetricName strips the _total suffix that Prometheus client libraries
// append to counter metric names in the exposition format.
func normalizeMetricName(name string) string {
	return strings.TrimSuffix(name, "_total")
}

func (c *Check) submitMetrics(s sender.Sender, families []prometheus.MetricFamily) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	currentGaugeKeys := make(map[string]struct{})

	for _, mf := range families {
		def, ok := c.metrics[normalizeMetricName(mf.Name)]
		if !ok {
			continue
		}

		// Aggregate cumulative counter values by their low-cardinality tag set,
		// dropping the high-cardinality `path` label. The driver never removes its
		// Prometheus series, so the per-context sum stays monotonic and
		// MonotonicCount derives a correct delta — without caching anything per
		// series, which would grow unbounded on ephemeral (per-pod) paths.
		counterSums := make(map[string]float64)
		counterTags := make(map[string][]string)

		// Same aggregation for the COAT counter: sum cumulative values per
		// preserved-tag context (endpoint + ordered tagValues) so prevValues is
		// keyed by the bounded context instead of the full label set (incl.
		// path), which would leak on ephemeral per-pod paths.
		coatCounterSums := make(map[string]float64)
		coatCounterTags := make(map[string][]string)

		for _, sample := range mf.Samples {
			switch def.kind {
			case counterMetric:
				tags := clientTags(sample.Metric)
				key := strings.Join(tags, "|")
				counterSums[key] += sample.Value
				counterTags[key] = tags
			case gaugeMetric:
				s.Gauge(metricNs+def.ddName, sample.Value, "", labelsToTags(sample.Metric))
			}

			if def.coat != nil {
				tagValues := make([]string, len(def.coat.tagKeys))
				for i, k := range def.coat.tagKeys {
					tagValues[i] = sample.Metric[k]
				}

				switch def.coat.kind {
				case counterMetric:
					key := c.endpointCoatKey(def.ddName, tagValues)
					coatCounterSums[key] += sample.Value
					coatCounterTags[key] = tagValues
				case gaugeMetric:
					// Keyed by COAT series identity; see sharedState (no cross-endpoint aggregation).
					key := coatSeriesKey(def.ddName, tagValues)
					currentGaugeKeys[key] = struct{}{}
					c.state.gaugeKeys[key] = gaugeSeries{
						coat:      def.coat,
						tagValues: slicesClone(tagValues),
					}
					def.coat.gauge.Set(sample.Value, tagValues...)
				}
			}

			if def.coatHist != nil {
				// Reduce the histogram to count/sum counters; ignore _bucket and
				// any other series (see coatHistogram). The raw series name is
				// kept in the __name__ label after the parser trims the family.
				rawName := sample.Metric["__name__"]
				var target telemetry.Counter
				switch {
				case strings.HasSuffix(rawName, "_count"):
					target = def.coatHist.count
				case strings.HasSuffix(rawName, "_sum"):
					target = def.coatHist.sum
				}
				if target != nil {
					tagValues := make([]string, len(def.coatHist.tagKeys))
					for i, k := range def.coatHist.tagKeys {
						tagValues[i] = sample.Metric[k]
					}
					c.addCounterDeltaLocked(target, c.endpointSeriesKey(rawName, sample.Metric), sample.Value, tagValues)
				}
			}
		}

		for key, sum := range counterSums {
			s.MonotonicCount(metricNs+def.ddName+".count", sum, "", counterTags[key])
		}

		if def.coat != nil && def.coat.kind == counterMetric {
			for key, sum := range coatCounterSums {
				c.addCounterDeltaLocked(def.coat.counter, key, sum, coatCounterTags[key])
			}
		}
	}

	c.deleteStaleGaugeSeriesLocked(currentGaugeKeys)
}

// addCounterDeltaLocked mirrors a cumulative Prometheus counter into a COAT
// counter by adding only the increment since the last scrape. A negative delta
// means the source counter was reset (e.g. driver restart), so the new value is
// added as-is instead of subtracting. The caller must hold c.state.mu.
func (c *Check) addCounterDeltaLocked(counter telemetry.Counter, key string, value float64, tagValues []string) {
	prev := c.state.prevValues[key]
	delta := value - prev
	if delta < 0 {
		delta = value
	}
	c.state.prevValues[key] = value
	counter.Add(delta, tagValues...)
}

// deleteAllGaugeSeries clears all tracked gauge series (used on cancel or scrape failure).
func (c *Check) deleteAllGaugeSeries() {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	c.deleteStaleGaugeSeriesLocked(nil)
}

// deleteStaleGaugeSeriesLocked deletes tracked gauge series absent from the latest
// scrape (currentGaugeKeys); a nil set deletes all. Caller must hold c.state.mu.
func (c *Check) deleteStaleGaugeSeriesLocked(currentGaugeKeys map[string]struct{}) {
	for key, series := range c.state.gaugeKeys {
		if _, ok := currentGaugeKeys[key]; ok {
			continue
		}
		delete(c.state.gaugeKeys, key)
		series.coat.gauge.Delete(series.tagValues...)
	}
}

func (c *Check) endpointSeriesKey(name string, labels prometheus.Metric) string {
	return c.config.OpenmetricsEndpoint + "|" + seriesKey(name, labels)
}

// endpointCoatKey builds the counter delta-cache key for a COAT counter from its
// preserved-tag context (ordered tagValues) rather than the full source label
// set. This bounds prevValues by the low-cardinality context (e.g. type×status)
// instead of growing per ephemeral path.
func (c *Check) endpointCoatKey(name string, tagValues []string) string {
	return c.config.OpenmetricsEndpoint + "|" + coatSeriesKey(name, tagValues)
}

func coatSeriesKey(name string, tagValues []string) string {
	return name + "|" + strings.Join(tagValues, "|")
}

func slicesClone(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// seriesKey builds a unique key for a metric time series from its name and
// all label values, used to track previous values for delta computation.
func seriesKey(name string, labels prometheus.Metric) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if k == "__name__" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(name)
	for _, k := range keys {
		b.WriteByte('|')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}

func labelsToTags(m prometheus.Metric) []string {
	tags := make([]string, 0, len(m))
	for k, v := range m {
		if k == "__name__" {
			continue
		}
		tags = append(tags, k+":"+v)
	}
	return tags
}

// pathLabel is the Prometheus label carrying the volume mount target (or, on
// success, the publisher-specific VolumePath). For per-pod publishers and any
// failed/unsupported mount it embeds the kubelet target path (with the pod UID),
// so its cardinality is unbounded and it must not become a Datadog tag.
const pathLabel = "path"

// clientTags converts Prometheus labels to Datadog tags for the client-facing
// (piste A) metrics, dropping the high-cardinality path label and returning a
// sorted slice usable as a stable grouping key. Dropping path collapses the
// per-path source series into the low-cardinality context (e.g. type, status)
// that matches the COAT preserved-tag set; callers sum the cumulative values
// per context before submitting, which stays monotonic because the driver never
// removes its Prometheus series.
func clientTags(m prometheus.Metric) []string {
	tags := slices.DeleteFunc(labelsToTags(m), func(t string) bool {
		return strings.HasPrefix(t, pathLabel+":")
	})
	sort.Strings(tags)
	return tags
}
