// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
)

const nameLabel = "__name__"

type parsedMetric struct {
	Name    string
	Type    string
	Samples []parsedSample
}

type parsedSample struct {
	Name      string
	Labels    map[string]string
	Value     float64
	Timestamp int64
}

func parseMetrics(data []byte, rawLineFilter *regexp.Regexp, useOpenMetrics bool, trimCounterSuffix bool) ([]parsedMetric, int, error) {
	filtered, ignoredLines := filterRawLines(data, rawLineFilter)

	st := labels.NewSymbolTable()
	var parser textparse.Parser
	infoTypes := map[string]struct{}{}
	if useOpenMetrics {
		parser = textparse.NewOpenMetricsParser(filtered, st, textparse.WithOMParserSTSeriesSkipped())
	} else {
		filtered, infoTypes = sanitizePromInfoTypes(filtered)
		parser = textparse.NewPromParser(filtered, st, false)
	}

	var metrics []parsedMetric
	var current *parsedMetric
	var lbls labels.Labels

	for {
		entry, err := parser.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, ignoredLines, err
		}

		switch entry {
		case textparse.EntryType:
			name, typ := parser.Type()
			metricType := strings.ToLower(string(typ))
			metricName := string(name)
			if _, ok := infoTypes[metricName]; ok {
				metricType = "info"
			}
			if trimCounterSuffix {
				metricName = normalizeFamilyName(metricName, metricType)
			}
			metrics = append(metrics, parsedMetric{Name: metricName, Type: metricType})
			current = &metrics[len(metrics)-1]
		case textparse.EntrySeries:
			_, ts, value := parser.Series()
			parser.Labels(&lbls)
			rawName := lbls.Get(model.MetricNameLabel)
			if rawName == "" {
				continue
			}

			if current == nil || !sampleBelongsToFamily(rawName, current) {
				metricType := "unknown"
				metricName := rawName
				metrics = append(metrics, parsedMetric{Name: metricName, Type: metricType})
				current = &metrics[len(metrics)-1]
			}

			sampleLabels := make(map[string]string, lbls.Len())
			lbls.Range(func(label labels.Label) {
				sampleLabels[label.Name] = label.Value
			})

			sample := parsedSample{
				Name:   rawName,
				Labels: sampleLabels,
				Value:  value,
			}
			if ts != nil {
				sample.Timestamp = *ts
			}
			current.Samples = append(current.Samples, sample)
		}
	}

	return dropEmptyMetrics(metrics), ignoredLines, nil
}

func filterRawLines(data []byte, filter *regexp.Regexp) ([]byte, int) {
	lines := bytes.Split(data, []byte{'\n'})
	filtered := make([][]byte, 0, len(lines))
	ignored := 0
	for _, line := range lines {
		line = bytes.TrimRight(bytes.TrimLeft(line, " \t"), "\r")
		if filter != nil && filter.Match(line) {
			ignored++
			continue
		}
		filtered = append(filtered, line)
	}
	return bytes.Join(filtered, []byte{'\n'}), ignored
}

func sanitizePromInfoTypes(data []byte) ([]byte, map[string]struct{}) {
	lines := bytes.Split(data, []byte{'\n'})
	infoTypes := map[string]struct{}{}
	for i, line := range lines {
		fields := bytes.Fields(line)
		if len(fields) == 4 &&
			string(fields[0]) == "#" &&
			string(fields[1]) == "TYPE" &&
			string(fields[3]) == "info" {
			infoTypes[string(fields[2])] = struct{}{}
			lines[i] = bytes.Join([][]byte{fields[0], fields[1], fields[2], []byte("gauge")}, []byte(" "))
		}
	}
	if len(infoTypes) == 0 {
		return data, nil
	}
	return bytes.Join(lines, []byte{'\n'}), infoTypes
}

func normalizeFamilyName(name, metricType string) string {
	if metricType == "counter" {
		return strings.TrimSuffix(name, "_total")
	}
	return name
}

func sampleBelongsToFamily(sampleName string, metric *parsedMetric) bool {
	if sampleName == metric.Name {
		return true
	}

	switch metric.Type {
	case "counter":
		return sampleName == metric.Name+"_total"
	case "histogram":
		return sampleName == metric.Name+"_bucket" || sampleName == metric.Name+"_sum" || sampleName == metric.Name+"_count"
	case "summary":
		return sampleName == metric.Name+"_sum" || sampleName == metric.Name+"_count"
	default:
		return false
	}
}

func dropEmptyMetrics(metrics []parsedMetric) []parsedMetric {
	out := metrics[:0]
	for _, metric := range metrics {
		if len(metric.Samples) > 0 {
			out = append(out, metric)
		}
	}
	return out
}
