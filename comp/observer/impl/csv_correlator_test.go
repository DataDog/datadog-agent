// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// csvAnomaly is a parsed row from the live extract CSV.
type csvAnomaly struct {
	timestamp    int64
	detector     string // "scanmw" or "scanwelch"
	metricName   string // e.g. "containerd.mem.cache:avg"
	rawTitle     string
	rawMessage   string
}

// parseExtractCSV reads the live Datadog event extract CSV and returns anomalies
// sorted by timestamp ascending. Expected columns: Date, Title, Message, Source.
// Title format: "Passthrough[scanmw]: metric.name:agg"
func parseExtractCSV(path string) ([]csvAnomaly, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV has no data rows")
	}

	titleRe := regexp.MustCompile(`^Passthrough\[(scanmw|scanwelch)\]:\s+(.+)$`)

	var out []csvAnomaly
	for _, row := range records[1:] { // skip header
		if len(row) < 3 {
			continue
		}
		dateStr, title := row[0], row[1]
		m := titleRe.FindStringSubmatch(title)
		if m == nil {
			continue // not a passthrough detector row
		}

		t, err := time.Parse(time.RFC3339Nano, dateStr)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05.000Z", dateStr)
			if err != nil {
				continue
			}
		}

		out = append(out, csvAnomaly{
			timestamp:  t.Unix(),
			detector:   m[1],
			metricName: m[2],
			rawTitle:   title,
			rawMessage: row[2],
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].timestamp < out[j].timestamp
	})
	return out, nil
}

// toObserverAnomaly converts a csvAnomaly to the observer.Anomaly type.
func toObserverAnomaly(a csvAnomaly) observer.Anomaly {
	parts := strings.SplitN(a.metricName, ":", 2)
	name := parts[0]
	agg := observer.AggregateAverage
	if len(parts) == 2 {
		switch parts[1] {
		case "count":
			agg = observer.AggregateCount
		case "sum":
			agg = observer.AggregateSum
		case "min":
			agg = observer.AggregateMin
		case "max":
			agg = observer.AggregateMax
		}
	}

	return observer.Anomaly{
		Type:         observer.AnomalyTypeMetric,
		DetectorName: a.detector,
		Title:        a.rawTitle,
		Description:  a.rawMessage,
		Timestamp:    a.timestamp,
		Source: observer.AnomalySource{
			Namespace: "live",
			Name:      name,
			Aggregate: agg,
		},
		SourceSeriesID: observer.SeriesID(fmt.Sprintf("live|%s|%s", a.metricName, a.detector)),
	}
}

// csvCorrelationResult is the JSON-serializable result.
type csvCorrelationResult struct {
	Pattern     string   `json:"pattern"`
	Title       string   `json:"title"`
	FirstSeen   int64    `json:"first_seen"`
	LastUpdated int64    `json:"last_updated"`
	NumAnoms    int      `json:"num_anomalies"`
	Metrics     []string `json:"metrics"`
}

// TestCSVThroughTimeCluster reads a live extract CSV, feeds anomalies through
// TimeClusterCorrelator using the same step-advance logic as the engine, and
// writes correlation results to /tmp/csv-time-cluster-results.json.
//
// Run: go test -run TestCSVThroughTimeCluster -v ./comp/observer/impl/ -csv /path/to/extract.csv
//
// If -csv is not provided, it defaults to the known extract location.
func TestCSVThroughTimeCluster(t *testing.T) {
	csvPath := os.Getenv("CSV_PATH")
	if csvPath == "" {
		csvPath = "/Users/ella.taira/Downloads/extract-2026-03-26T17_09_16.460Z.csv"
	}

	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		t.Skipf("CSV not found at %s — set CSV_PATH env var", csvPath)
	}

	anomalies, err := parseExtractCSV(csvPath)
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}
	t.Logf("Parsed %d passthrough anomalies from CSV", len(anomalies))

	// Build correlator with default config (same as production).
	correlator := NewTimeClusterCorrelator(DefaultTimeClusterConfig())

	// Mirror the engine's advance pattern: the live observer advances the
	// correlator every second (via the scheduler), not just when anomalies
	// arrive. Between anomaly bursts, the correlator still gets Advance()
	// calls that tick the clock forward and evict stale clusters.
	//
	// We simulate this by advancing second-by-second from minTS to maxTS,
	// feeding anomalies at their corresponding seconds.
	var allCorrelations []observer.ActiveCorrelation
	seen := map[string]bool{}

	// Index anomalies by timestamp for O(1) lookup.
	anomsByTS := map[int64][]observer.Anomaly{}
	for _, a := range anomalies {
		obs := toObserverAnomaly(a)
		anomsByTS[obs.Timestamp] = append(anomsByTS[obs.Timestamp], obs)
	}

	minTS := anomalies[0].timestamp
	maxTS := anomalies[len(anomalies)-1].timestamp
	t.Logf("Replaying %d seconds (%d→%d)", maxTS-minTS+1, minTS, maxTS)

	for sec := minTS; sec <= maxTS; sec++ {
		// Feed any anomalies at this second.
		if batch, ok := anomsByTS[sec]; ok {
			for _, obs := range batch {
				correlator.ProcessAnomaly(obs)
			}
		}
		// Advance every second, same as the engine scheduler.
		correlator.Advance(sec)
		for _, c := range correlator.ActiveCorrelations() {
			if !seen[c.Pattern] {
				seen[c.Pattern] = true
				allCorrelations = append(allCorrelations, c)
			}
		}
	}

	t.Logf("TimeCluster produced %d correlation patterns", len(allCorrelations))

	// Build output in testbench headless JSON format so the scorer can read it.
	var periods []map[string]any
	for _, c := range allCorrelations {
		periods = append(periods, map[string]any{
			"pattern":      c.Pattern,
			"period_start": c.FirstSeen,
			"period_end":   c.LastUpdated,
			"title":        c.Title,
			"message":      fmt.Sprintf("%d anomalies across %d series", len(c.Anomalies), len(c.MemberSeriesIDs)),
			"tags":         []string{"source:csv-replay", "pattern:" + c.Pattern},
		})
	}

	output := map[string]any{
		"metadata": map[string]any{
			"scenario":              "food_delivery_redis",
			"timeline_start":        minTS,
			"timeline_end":          maxTS,
			"detectors_enabled":     []string{"scanmw", "scanwelch"},
			"correlators_enabled":   []string{"time_cluster"},
			"total_anomaly_periods": len(periods),
		},
		"anomaly_periods": periods,
	}

	outPath := "/tmp/observer-eval-food_delivery_redis.json"
	data, _ := json.MarshalIndent(output, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		t.Fatalf("failed to write output: %v", err)
	}

	t.Logf("Wrote %d periods to %s", len(periods), outPath)

	for _, c := range allCorrelations {
		metrics := make(map[string]bool)
		for _, a := range c.Anomalies {
			metrics[fmt.Sprintf("%s:%s", a.Source.Name, aggregateStr(a.Source.Aggregate))] = true
		}
		t.Logf("Cluster %s (%d anomalies, %d metrics): %s",
			c.Pattern, len(c.Anomalies), len(metrics), c.Title)
	}
}

func aggregateStr(a observer.Aggregate) string {
	switch a {
	case observer.AggregateAverage:
		return "avg"
	case observer.AggregateCount:
		return "count"
	case observer.AggregateSum:
		return "sum"
	case observer.AggregateMin:
		return "min"
	case observer.AggregateMax:
		return "max"
	default:
		return "none"
	}
}
