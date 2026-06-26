// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

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

type parsedMetricHandler func(parsedMetric) (bool, error)

type streamParseResult struct {
	bytesRead    int64
	ignoredLines int
}

func walkParsedMetrics(data []byte, rawLineFilter *regexp.Regexp, useOpenMetrics bool, trimCounterSuffix bool, handler parsedMetricHandler) (int, error) {
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

	var current parsedMetric
	hasCurrent := false
	materialize := true
	var lbls labels.Labels
	flushCurrent := func() error {
		if !hasCurrent {
			return nil
		}
		if len(current.Samples) == 0 {
			hasCurrent = false
			return nil
		}
		keepMaterializing, err := handler(current)
		if err != nil {
			return err
		}
		materialize = keepMaterializing
		current = parsedMetric{}
		hasCurrent = false
		return nil
	}

	for {
		entry, err := parser.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ignoredLines, err
		}

		switch entry {
		case textparse.EntryType:
			if err := flushCurrent(); err != nil {
				return ignoredLines, err
			}
			if !materialize {
				continue
			}
			name, typ := parser.Type()
			metricType := strings.ToLower(string(typ))
			metricName := string(name)
			if _, ok := infoTypes[metricName]; ok {
				metricType = "info"
			}
			if trimCounterSuffix {
				metricName = normalizeFamilyName(metricName, metricType)
			}
			current = parsedMetric{Name: metricName, Type: metricType}
			hasCurrent = true
		case textparse.EntrySeries:
			if !materialize {
				continue
			}
			_, ts, value := parser.Series()
			parser.Labels(&lbls)
			rawName := lbls.Get(model.MetricNameLabel)
			if rawName == "" {
				continue
			}

			if !hasCurrent || !sampleBelongsToFamily(rawName, &current) {
				if err := flushCurrent(); err != nil {
					return ignoredLines, err
				}
				metricType := "unknown"
				metricName := rawName
				current = parsedMetric{Name: metricName, Type: metricType}
				hasCurrent = true
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

	if err := flushCurrent(); err != nil {
		return ignoredLines, err
	}
	return ignoredLines, nil
}

func walkPrometheusTextSamples(r io.Reader, trimCounterSuffix bool, shouldMaterialize func([]byte, map[string]string) bool, handler func(parsedSample, map[string]string) error) (streamParseResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	result := streamParseResult{}
	metricTypes := map[string]string{}
	for scanner.Scan() {
		line := scanner.Bytes()
		result.bytesRead += int64(len(line)) + 1
		line = bytes.TrimRight(bytes.TrimLeft(line, " \t"), "\r")
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte("#")) {
			name, typ, ok, err := parsePrometheusTypeLine(line)
			if err != nil {
				return result, err
			}
			if !ok {
				continue
			}
			if trimCounterSuffix {
				name = normalizeFamilyName(name, typ)
			}
			metricTypes[name] = typ
			continue
		}
		sampleName, err := prometheusSampleName(line)
		if err != nil {
			return result, err
		}
		materialize := shouldMaterialize(sampleName, metricTypes)
		sample, err := parsePrometheusSampleLine(line, materialize)
		if err != nil {
			return result, err
		}
		if !materialize {
			continue
		}
		if err := handler(sample, metricTypes); err != nil {
			return result, err
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	return result, nil
}

func prometheusSampleName(line []byte) ([]byte, error) {
	nameEnd := bytes.IndexAny(line, "{ \t")
	if nameEnd < 0 {
		return nil, fmt.Errorf("invalid Prometheus sample line %q: missing value", line)
	}
	if !validPrometheusMetricNameBytes(line[:nameEnd]) {
		return nil, fmt.Errorf("invalid Prometheus sample line %q: invalid metric name", line)
	}
	return line[:nameEnd], nil
}

func parsePrometheusTypeLine(line []byte) (string, string, bool, error) {
	fields := bytes.Fields(line)
	if len(fields) < 2 || string(fields[0]) != "#" || string(fields[1]) != "TYPE" {
		return "", "", false, nil
	}
	if len(fields) != 4 {
		return "", "", false, fmt.Errorf("invalid Prometheus TYPE line %q", line)
	}
	typ := strings.ToLower(string(fields[3]))
	if !validPrometheusType(typ) {
		return "", "", false, fmt.Errorf("invalid Prometheus TYPE line %q: invalid type", line)
	}
	name := string(fields[2])
	if !validPrometheusMetricName(name) {
		return "", "", false, fmt.Errorf("invalid Prometheus TYPE line %q: invalid metric name", line)
	}
	return name, typ, true, nil
}

func validPrometheusType(typ string) bool {
	switch typ {
	case "counter", "gauge", "histogram", "summary", "untyped", "info":
		return true
	default:
		return false
	}
}

func validPrometheusMetricName(name string) bool {
	return validPrometheusMetricNameBytes([]byte(name))
}

func validPrometheusMetricNameBytes(name []byte) bool {
	if len(name) == 0 {
		return false
	}
	for i, b := range name {
		switch {
		case b == ':' || b == '_':
		case b >= 'A' && b <= 'Z':
		case b >= 'a' && b <= 'z':
		case i > 0 && b >= '0' && b <= '9':
		default:
			return false
		}
	}
	return true
}

func validPrometheusLabelNameBytes(name []byte) bool {
	if len(name) == 0 {
		return false
	}
	for i, b := range name {
		switch {
		case b == '_':
		case b >= 'A' && b <= 'Z':
		case b >= 'a' && b <= 'z':
		case i > 0 && b >= '0' && b <= '9':
		default:
			return false
		}
	}
	return true
}

func validPrometheusFloat(value []byte) bool {
	if len(value) == 0 || bytes.ContainsAny(value, "_") {
		return false
	}
	lower := bytes.ToLower(value)
	return !bytes.HasPrefix(lower, []byte("0x")) &&
		!bytes.HasPrefix(lower, []byte("+0x")) &&
		!bytes.HasPrefix(lower, []byte("-0x"))
}

func validPrometheusTimestamp(value []byte) bool {
	if len(value) == 0 {
		return false
	}
	for i, b := range value {
		if b == '-' && i == 0 {
			continue
		}
		if b < '0' || b > '9' {
			return false
		}
	}
	return len(value) > 1 || value[0] != '-'
}

func parsePrometheusSampleLine(line []byte, materializeLabels bool) (parsedSample, error) {
	nameEnd := bytes.IndexAny(line, "{ \t")
	if nameEnd < 0 {
		return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: missing value", line)
	}
	var name string
	if materializeLabels {
		name = string(line[:nameEnd])
	}
	if !validPrometheusMetricNameBytes(line[:nameEnd]) {
		return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: invalid metric name", line)
	}
	rest := bytes.TrimLeft(line[nameEnd:], " \t")
	var labels map[string]string
	if materializeLabels {
		labels = map[string]string{nameLabel: name}
	}
	if len(rest) > 0 && rest[0] == '{' {
		end := findLabelSetEnd(rest)
		if end < 0 {
			return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: unterminated label set", line)
		}
		if err := parsePrometheusLabelsInto(labels, rest[1:end]); err != nil {
			return parsedSample{}, err
		}
		rest = bytes.TrimLeft(rest[end+1:], " \t")
	}
	if len(rest) == 0 {
		return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: missing value", line)
	}
	valueEnd := bytes.IndexAny(rest, " \t")
	var rawValue []byte
	if valueEnd < 0 {
		rawValue = rest
		rest = nil
	} else {
		rawValue = rest[:valueEnd]
		rest = bytes.TrimLeft(rest[valueEnd:], " \t")
	}
	if !validPrometheusFloat(rawValue) {
		return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: invalid value", line)
	}
	value, err := strconv.ParseFloat(string(rawValue), 64)
	if err != nil {
		return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: invalid value: %w", line, err)
	}
	sample := parsedSample{
		Name:   name,
		Labels: labels,
		Value:  value,
	}
	if len(rest) > 0 {
		timestampEnd := bytes.IndexAny(rest, " \t")
		rawTimestamp := rest
		if timestampEnd >= 0 {
			rawTimestamp = rest[:timestampEnd]
		}
		if !validPrometheusTimestamp(rawTimestamp) {
			return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: invalid timestamp", line)
		}
		timestamp, err := strconv.ParseInt(string(rawTimestamp), 10, 64)
		if err != nil {
			return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: invalid timestamp: %w", line, err)
		}
		sample.Timestamp = timestamp
		if timestampEnd >= 0 {
			trailing := bytes.TrimLeft(rest[timestampEnd:], " \t")
			if len(trailing) > 0 {
				return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: unexpected data after timestamp", line)
			}
		}
	}
	return sample, nil
}

func findLabelSetEnd(data []byte) int {
	escaped := false
	inQuotes := false
	for i, b := range data {
		if escaped {
			escaped = false
			continue
		}
		switch b {
		case '\\':
			escaped = inQuotes
		case '"':
			inQuotes = !inQuotes
		case '}':
			if !inQuotes {
				return i
			}
		}
	}
	return -1
}

func parsePrometheusLabelsInto(labels map[string]string, data []byte) error {
	var seen [8][]byte
	seenCount := 0
	var seenOverflow map[string]struct{}
	for len(data) > 0 {
		data = bytes.TrimLeft(data, " \t")
		if len(data) == 0 {
			break
		}
		eq := bytes.IndexByte(data, '=')
		if eq <= 0 {
			return fmt.Errorf("invalid Prometheus label set %q: missing `=`", data)
		}
		name := bytes.TrimSpace(data[:eq])
		if !validPrometheusLabelNameBytes(name) {
			return fmt.Errorf("invalid Prometheus label set %q: invalid label name", data)
		}
		duplicate := false
		for i := 0; i < seenCount; i++ {
			if bytes.Equal(seen[i], name) {
				duplicate = true
				break
			}
		}
		if seenOverflow != nil {
			if _, ok := seenOverflow[string(name)]; ok {
				duplicate = true
			}
		}
		if duplicate {
			return fmt.Errorf("invalid Prometheus label set %q: duplicate label name", data)
		}
		if seenCount < len(seen) {
			seen[seenCount] = name
			seenCount++
		} else {
			if seenOverflow == nil {
				seenOverflow = make(map[string]struct{}, 1)
			}
			seenOverflow[string(name)] = struct{}{}
		}
		data = bytes.TrimLeft(data[eq+1:], " \t")
		if len(data) == 0 || data[0] != '"' {
			return fmt.Errorf("invalid Prometheus label set %q: missing quoted value", data)
		}
		var consumed int
		if labels == nil {
			var err error
			consumed, err = validatePrometheusLabelValue(data)
			if err != nil {
				return err
			}
		} else {
			value, parsedConsumed, err := parsePrometheusLabelValue(data)
			if err != nil {
				return err
			}
			labels[string(name)] = value
			consumed = parsedConsumed
		}
		data = bytes.TrimLeft(data[consumed:], " \t")
		if len(data) == 0 {
			break
		}
		if data[0] != ',' {
			return fmt.Errorf("invalid Prometheus label set %q: missing `,`", data)
		}
		data = data[1:]
	}
	return nil
}

func validatePrometheusLabelValue(data []byte) (int, error) {
	if len(data) == 0 || data[0] != '"' {
		return 0, fmt.Errorf("invalid Prometheus label value %q", data)
	}
	for i := 1; i < len(data); i++ {
		switch data[i] {
		case '"':
			if !utf8.Valid(data[1:i]) {
				return 0, fmt.Errorf("invalid Prometheus label value %q: invalid utf-8", data)
			}
			return i + 1, nil
		case '\\':
			if i+1 >= len(data) {
				return 0, fmt.Errorf("invalid Prometheus label value %q: trailing escape", data)
			}
			switch data[i+1] {
			case '\\', '"', 'n':
			default:
				return 0, fmt.Errorf("invalid Prometheus label value %q: invalid escape", data)
			}
			i++
		}
	}
	return 0, fmt.Errorf("invalid Prometheus label value %q: unterminated quote", data)
}

func parsePrometheusLabelValue(data []byte) (string, int, error) {
	if len(data) == 0 || data[0] != '"' {
		return "", 0, fmt.Errorf("invalid Prometheus label value %q", data)
	}
	var builder strings.Builder
	for i := 1; i < len(data); i++ {
		switch data[i] {
		case '"':
			if builder.Cap() == 0 {
				value := string(data[1:i])
				if !utf8.ValidString(value) {
					return "", 0, fmt.Errorf("invalid Prometheus label value %q: invalid utf-8", data)
				}
				return value, i + 1, nil
			}
			value := builder.String()
			if !utf8.ValidString(value) {
				return "", 0, fmt.Errorf("invalid Prometheus label value %q: invalid utf-8", data)
			}
			return value, i + 1, nil
		case '\\':
			if builder.Cap() == 0 {
				builder.Grow(len(data) - 2)
				builder.Write(data[1:i])
			}
			if i+1 >= len(data) {
				return "", 0, fmt.Errorf("invalid Prometheus label value %q: trailing escape", data)
			}
			i++
			switch data[i] {
			case 'n':
				builder.WriteByte('\n')
			case '\\':
				builder.WriteByte('\\')
			case '"':
				builder.WriteByte('"')
			default:
				return "", 0, fmt.Errorf("invalid Prometheus label value %q: invalid escape", data)
			}
		default:
			if builder.Cap() > 0 {
				builder.WriteByte(data[i])
			}
		}
	}
	return "", 0, fmt.Errorf("invalid Prometheus label value %q: unterminated quote", data)
}

func prometheusFamilyName(sampleName string, metricTypes map[string]string, trimCounterSuffix bool) string {
	if _, ok := metricTypes[sampleName]; ok {
		return sampleName
	}
	for _, suffix := range []string{"_bucket", "_sum", "_count"} {
		if strings.HasSuffix(sampleName, suffix) {
			family := strings.TrimSuffix(sampleName, suffix)
			if typ := metricTypes[family]; typ == "histogram" || typ == "summary" {
				return family
			}
		}
	}
	if trimCounterSuffix && strings.HasSuffix(sampleName, "_total") {
		family := strings.TrimSuffix(sampleName, "_total")
		if metricTypes[family] == "counter" {
			return family
		}
	}
	return sampleName
}

func prometheusFamilyNameBytes(sampleName []byte, metricTypes map[string]string, trimCounterSuffix bool) string {
	if _, ok := metricTypes[string(sampleName)]; ok {
		return string(sampleName)
	}
	for _, suffix := range []string{"_bucket", "_sum", "_count"} {
		if bytes.HasSuffix(sampleName, []byte(suffix)) {
			family := string(bytes.TrimSuffix(sampleName, []byte(suffix)))
			if typ := metricTypes[family]; typ == "histogram" || typ == "summary" {
				return family
			}
		}
	}
	if trimCounterSuffix && bytes.HasSuffix(sampleName, []byte("_total")) {
		family := string(bytes.TrimSuffix(sampleName, []byte("_total")))
		if metricTypes[family] == "counter" {
			return family
		}
	}
	return string(sampleName)
}

func filterRawLines(data []byte, filter *regexp.Regexp) ([]byte, int) {
	if filter == nil {
		return data, 0
	}
	lines := bytes.Split(data, []byte{'\n'})
	filtered := make([][]byte, 0, len(lines))
	ignored := 0
	for _, line := range lines {
		line = bytes.TrimRight(bytes.TrimLeft(line, " \t"), "\r")
		if filter.Match(line) {
			ignored++
			continue
		}
		filtered = append(filtered, line)
	}
	return bytes.Join(filtered, []byte{'\n'}), ignored
}

func sanitizePromInfoTypes(data []byte) ([]byte, map[string]struct{}) {
	if !bytes.Contains(data, []byte("info")) {
		return data, nil
	}
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
