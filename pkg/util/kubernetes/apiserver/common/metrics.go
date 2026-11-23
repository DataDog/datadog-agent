// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"

	"k8s.io/client-go/discovery"

	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

// maxMetricLineSize bounds the size of a single line read from the /metrics endpoint.
// It only needs to be large enough to hold one metric sample line (name + labels), not
// the whole response.
const maxMetricLineSize = 100 * 1024

// FetchAPIServerMetricFamily queries the API server's /metrics endpoint once (no retry) and
// returns the first parsed metric family present in the response, in the order specified by
// metricNames. It returns nil if none of the requested families are present.
//
// The API server's /metrics endpoint can return a very large payload (tens of MiB), most of
// which is irrelevant to the metric families we care about. To avoid buffering the whole
// response in memory, the response body is streamed and scanned line by line, keeping only
// the lines belonging to the requested metric families before handing them to the parser.
func FetchAPIServerMetricFamily(ctx context.Context, discoveryClient discovery.DiscoveryInterface, metricNames ...string) (*prometheus.MetricFamily, error) {
	body, err := discoveryClient.RESTClient().Get().AbsPath(apiServerMetricsPath).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query /metrics endpoint: %w", err)
	}
	defer body.Close()

	metricsData, err := filterMetricLines(body, metricNames)
	if err != nil {
		return nil, fmt.Errorf("failed to read /metrics endpoint: %w", err)
	}

	families, err := prometheus.ParseMetrics(metricsData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse /metrics endpoint: %w", err)
	}

	return findMetricFamily(families, metricNames...), nil
}

// filterMetricLines scans r line by line and returns only the lines that are relevant to
// metricNames: their "# HELP"/"# TYPE" comment lines and their sample lines. This keeps peak
// memory proportional to the size of the requested metric families rather than to the size
// of the whole /metrics response.
func filterMetricLines(r io.Reader, metricNames []string) ([]byte, error) {
	var buf bytes.Buffer

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxMetricLineSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if !lineMatchesAnyMetric(line, metricNames) {
			continue
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// lineMatchesAnyMetric reports whether line belongs to one of metricNames: either a
// "# HELP <name> ..."/"# TYPE <name> ..." comment line, or a sample line whose metric name
// (the part of the line before "{" or the first space) is exactly one of metricNames.
func lineMatchesAnyMetric(line []byte, metricNames []string) bool {
	trimmed := bytes.TrimLeft(line, " \t")

	if bytes.HasPrefix(trimmed, []byte("# HELP ")) || bytes.HasPrefix(trimmed, []byte("# TYPE ")) {
		fields := bytes.Fields(trimmed)
		if len(fields) < 3 {
			return false
		}
		return matchesAny(string(fields[2]), metricNames)
	}

	if len(trimmed) == 0 || trimmed[0] == '#' {
		return false
	}

	end := bytes.IndexAny(trimmed, "{ ")
	if end == -1 {
		end = len(trimmed)
	}

	return matchesAny(string(trimmed[:end]), metricNames)
}

func matchesAny(name string, metricNames []string) bool {
	return slices.Contains(metricNames, name)
}

func findMetricFamily(families []prometheus.MetricFamily, metricNames ...string) *prometheus.MetricFamily {
	for _, metricName := range metricNames {
		for i := range families {
			if families[i].Name == metricName {
				return &families[i]
			}
		}
	}

	return nil
}
