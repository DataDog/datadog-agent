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
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
)

const nameLabel = "__name__"
const openMetricsStreamMaxLineSize = 1024 * 1024

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
	if !useOpenMetrics && requiresPrometheusCompatibilityParser(filtered) {
		return ignoredLines, walkPrometheusMetricFamilies(bytes.NewReader(filtered), trimCounterSuffix, handler)
	}

	st := labels.NewSymbolTable()
	var parser textparse.Parser
	if useOpenMetrics {
		filtered = sanitizeFloatOverflows(filtered)
		parser = textparse.NewOpenMetricsParser(filtered, st, textparse.WithOMParserSTSeriesSkipped())
	} else {
		parser = textparse.NewPromParser(filtered, st, false)
	}

	var current parsedMetric
	currentRawName := ""
	hasCurrent := false
	materialize := true
	handledMetrics := 0
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
		handledMetrics++
		materialize = keepMaterializing
		current = parsedMetric{}
		currentRawName = ""
		hasCurrent = false
		return nil
	}

	for {
		entry, err := parser.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, strconv.ErrRange) {
				remainingSkips := handledMetrics
				fallbackHandler := func(metric parsedMetric) (bool, error) {
					if remainingSkips > 0 {
						remainingSkips--
						return true, nil
					}
					return handler(metric)
				}
				var fallbackErr error
				if useOpenMetrics {
					normalized := sanitizeFloatOverflowsAll(filtered)
					if !bytes.Equal(normalized, filtered) {
						_, fallbackErr = walkParsedMetrics(normalized, nil, true, trimCounterSuffix, fallbackHandler)
					} else {
						fallbackErr = err
					}
				} else {
					fallbackErr = walkPrometheusMetricFamilies(bytes.NewReader(filtered), trimCounterSuffix, fallbackHandler)
				}
				if fallbackErr == nil && remainingSkips == 0 {
					return ignoredLines, nil
				}
			}
			return ignoredLines, err
		}

		switch entry {
		case textparse.EntryType:
			name, typ := parser.Type()
			metricType := strings.ToLower(string(typ))
			rawMetricName := string(name)
			metricName := rawMetricName
			if trimCounterSuffix {
				metricName = normalizeFamilyName(metricName, metricType)
			}
			if hasCurrent && currentRawName == rawMetricName {
				current.Name = metricName
				current.Type = metricType
				continue
			}
			if err := flushCurrent(); err != nil {
				return ignoredLines, err
			}
			if !materialize {
				continue
			}
			current = parsedMetric{Name: metricName, Type: metricType}
			currentRawName = rawMetricName
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

func walkPrometheusTextSamples(r io.Reader, trimCounterSuffix bool, shouldMaterialize func([]byte, map[string]string) bool, typeHandler func(string, string, string) error, handler func(parsedSample, map[string]string) error) (streamParseResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	result := streamParseResult{}
	metricTypes := map[string]string{}
	metricFamilies := map[string]string{}
	for scanner.Scan() {
		line := scanner.Bytes()
		result.bytesRead += int64(len(line)) + 1
		line = bytes.TrimSpace(line)
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
			rawName := name
			if trimCounterSuffix {
				name = normalizeFamilyName(rawName, typ)
			}
			if previousFamily, ok := metricFamilies[rawName]; ok && previousFamily != name {
				delete(metricTypes, previousFamily)
			}
			metricFamilies[rawName] = name
			metricTypes[name] = typ
			if typeHandler != nil {
				if err := typeHandler(rawName, name, typ); err != nil {
					return result, err
				}
			}
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

func walkPrometheusMetricFamilies(r io.Reader, trimCounterSuffix bool, handler parsedMetricHandler) error {
	pending := parsedMetric{}
	pendingRawName := ""
	declarationRawName := ""
	declarationFamilyName := ""
	materialize := true
	flushPending := func() error {
		if len(pending.Samples) == 0 {
			return nil
		}
		keepMaterializing, err := handler(pending)
		if err != nil {
			return err
		}
		materialize = keepMaterializing
		pending = parsedMetric{}
		pendingRawName = ""
		return nil
	}

	_, err := walkPrometheusTextSamples(r, trimCounterSuffix, func([]byte, map[string]string) bool {
		return materialize
	}, func(rawName string, familyName string, metricType string) error {
		if len(pending.Samples) > 0 && pendingRawName == rawName {
			pending.Name = familyName
			pending.Type = metricType
		} else if len(pending.Samples) > 0 {
			if err := flushPending(); err != nil {
				return err
			}
		}
		declarationRawName = rawName
		declarationFamilyName = familyName
		return nil
	}, func(sample parsedSample, metricTypes map[string]string) error {
		familyName := prometheusFamilyName(sample.Name, metricTypes, trimCounterSuffix)
		if len(pending.Samples) > 0 && pending.Name != familyName {
			if err := flushPending(); err != nil {
				return err
			}
		}
		if !materialize {
			return nil
		}
		if len(pending.Samples) == 0 {
			pending.Name = familyName
			if declarationFamilyName == familyName {
				pendingRawName = declarationRawName
			} else {
				pendingRawName = sample.Name
			}
		}
		pending.Type = metricTypes[familyName]
		if pending.Type == "" {
			pending.Type = "unknown"
		}
		pending.Samples = append(pending.Samples, sample)
		return nil
	})
	if err != nil {
		return err
	}
	return flushPending()
}

func walkOpenMetricsTextSamples(r io.Reader, trimCounterSuffix bool, shouldMaterialize func(string) bool, handler func(parsedSample, parsedMetric) error) (streamParseResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), openMetricsStreamMaxLineSize)

	result := streamParseResult{}
	currentMetric := parsedMetric{}
	hasCurrentMetric := false
	seenEOF := false
	for scanner.Scan() {
		line := bytes.TrimRight(scanner.Bytes(), "\r")
		result.bytesRead += int64(len(line)) + 1

		if seenEOF {
			return result, errors.New("unexpected data after # EOF")
		}
		if bytes.Equal(line, []byte("# EOF")) {
			seenEOF = true
			continue
		}
		if len(line) == 0 {
			return result, fmt.Errorf("invalid OpenMetrics line %q", line)
		}
		if line[0] == ' ' || line[0] == '\t' {
			return result, fmt.Errorf("invalid OpenMetrics line %q: unexpected leading whitespace", line)
		}
		if bytes.HasPrefix(line, []byte("#")) {
			name, typ, ok, err := parseOpenMetricsTypeLine(line)
			if err != nil {
				return result, err
			}
			if !ok {
				if err := validateOpenMetricsMetadataLine(line); err != nil {
					return result, err
				}
				continue
			}
			if trimCounterSuffix {
				name = normalizeFamilyName(name, typ)
			}
			currentMetric = parsedMetric{Name: name, Type: typ}
			hasCurrentMetric = true
			continue
		}

		rawNameBytes, rawName, err := openMetricsSampleName(line)
		if err != nil {
			return result, err
		}
		if hasCurrentMetric && openMetricsCreatedSeries(rawNameBytes, rawName, currentMetric) {
			continue
		}

		familyName := ""
		metric := parsedMetric{Type: "unknown"}
		if hasCurrentMetric && openMetricsSampleBelongsToFamily(rawNameBytes, rawName, &currentMetric) {
			familyName = currentMetric.Name
			metric = currentMetric
		} else {
			familyName = openMetricsSampleNameString(rawNameBytes, rawName)
			metric.Name = familyName
		}

		materialize := shouldMaterialize(familyName)
		sample, err := parseOpenMetricsSampleLine(line, materialize)
		if err != nil {
			return result, err
		}
		if !materialize {
			continue
		}
		if err := handler(sample, metric); err != nil {
			return result, err
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	if !seenEOF {
		return result, errors.New("data does not end with # EOF")
	}
	return result, nil
}

func parseOpenMetricsTypeLine(line []byte) (string, string, bool, error) {
	const prefix = "# TYPE "
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return "", "", false, nil
	}
	name, consumed, err := parseOpenMetricsIdentifier(line[len(prefix):])
	if err != nil {
		return "", "", false, fmt.Errorf("invalid OpenMetrics TYPE line %q: %w", line, err)
	}
	rest := line[len(prefix)+consumed:]
	if len(rest) == 0 || rest[0] != ' ' {
		return "", "", false, fmt.Errorf("invalid OpenMetrics TYPE line %q: missing type", line)
	}
	typ := string(rest[1:])
	if !validOpenMetricsType(typ) {
		return "", "", false, fmt.Errorf("invalid OpenMetrics TYPE line %q: invalid type", line)
	}
	return name, typ, true, nil
}

func validateOpenMetricsMetadataLine(line []byte) error {
	var prefix string
	switch {
	case bytes.HasPrefix(line, []byte("# HELP ")):
		prefix = "# HELP "
	case bytes.HasPrefix(line, []byte("# UNIT ")):
		prefix = "# UNIT "
	default:
		return nil
	}

	name, consumed, err := parseOpenMetricsIdentifier(line[len(prefix):])
	if err != nil {
		return fmt.Errorf("invalid OpenMetrics metadata line %q: %w", line, err)
	}
	rest := line[len(prefix)+consumed:]
	if len(rest) < 2 || rest[0] != ' ' {
		return fmt.Errorf("invalid OpenMetrics metadata line %q: missing text", line)
	}
	text := string(rest[1:])
	if !utf8.ValidString(text) {
		return fmt.Errorf("invalid OpenMetrics metadata line %q: invalid utf-8", line)
	}
	if prefix == "# UNIT " && !strings.HasSuffix(name, "_"+text) {
		return fmt.Errorf("invalid OpenMetrics metadata line %q: unit is not a metric suffix", line)
	}
	return nil
}

func validOpenMetricsType(typ string) bool {
	switch typ {
	case "counter", "gauge", "histogram", "gaugehistogram", "summary", "info", "stateset", "unknown":
		return true
	default:
		return false
	}
}

func openMetricsCreatedSeries(sampleNameBytes []byte, sampleName string, metric parsedMetric) bool {
	switch metric.Type {
	case "counter", "histogram", "summary":
		if sampleName != "" {
			return sampleName == metric.Name+"_created"
		}
		return bytesEqualStringWithSuffix(sampleNameBytes, metric.Name, "_created")
	default:
		return false
	}
}

func openMetricsSampleName(line []byte) ([]byte, string, error) {
	if len(line) == 0 {
		return nil, "", fmt.Errorf("invalid OpenMetrics sample line %q: missing metric name", line)
	}
	if line[0] == '{' {
		end := findLabelSetEnd(line)
		if end < 0 {
			return nil, "", fmt.Errorf("invalid OpenMetrics sample line %q: unterminated label set", line)
		}
		name, err := parseOpenMetricsLabelsInto(nil, line[1:end], true)
		if err != nil {
			return nil, "", err
		}
		if name == "" {
			return nil, "", fmt.Errorf("invalid OpenMetrics sample line %q: missing metric name", line)
		}
		return nil, name, nil
	}
	name, _, err := parseOpenMetricsIdentifierRaw(line)
	return name, "", err
}

func openMetricsSampleNameString(sampleNameBytes []byte, sampleName string) string {
	if sampleName != "" {
		return sampleName
	}
	return string(sampleNameBytes)
}

func openMetricsSampleBelongsToFamily(sampleNameBytes []byte, sampleName string, metric *parsedMetric) bool {
	if sampleName != "" {
		return sampleBelongsToFamily(sampleName, metric)
	}
	if bytesEqualString(sampleNameBytes, metric.Name) {
		return true
	}

	switch metric.Type {
	case "counter":
		return bytesEqualStringWithSuffix(sampleNameBytes, metric.Name, "_total")
	case "histogram":
		return bytesEqualStringWithSuffix(sampleNameBytes, metric.Name, "_bucket") ||
			bytesEqualStringWithSuffix(sampleNameBytes, metric.Name, "_sum") ||
			bytesEqualStringWithSuffix(sampleNameBytes, metric.Name, "_count")
	case "summary":
		return bytesEqualStringWithSuffix(sampleNameBytes, metric.Name, "_sum") ||
			bytesEqualStringWithSuffix(sampleNameBytes, metric.Name, "_count")
	default:
		return false
	}
}

func bytesEqualString(data []byte, value string) bool {
	if len(data) != len(value) {
		return false
	}
	for i := range data {
		if data[i] != value[i] {
			return false
		}
	}
	return true
}

func bytesEqualStringWithSuffix(data []byte, value string, suffix string) bool {
	if len(data) != len(value)+len(suffix) {
		return false
	}
	for i := 0; i < len(value); i++ {
		if data[i] != value[i] {
			return false
		}
	}
	for i := 0; i < len(suffix); i++ {
		if data[len(value)+i] != suffix[i] {
			return false
		}
	}
	return true
}

func parseOpenMetricsSampleLine(line []byte, materializeLabels bool) (parsedSample, error) {
	var name string
	var rest []byte
	var labels map[string]string
	if line[0] == '{' {
		end := findLabelSetEnd(line)
		if end < 0 {
			return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: unterminated label set", line)
		}
		if materializeLabels {
			labels = map[string]string{}
		}
		var err error
		name, err = parseOpenMetricsLabelsInto(labels, line[1:end], true)
		if err != nil {
			return parsedSample{}, err
		}
		if name == "" {
			return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: missing metric name", line)
		}
		if materializeLabels {
			labels[nameLabel] = name
		}
		rest = line[end+1:]
	} else {
		var consumed int
		if materializeLabels {
			parsedName, parsedConsumed, err := parseOpenMetricsIdentifier(line)
			if err != nil {
				return parsedSample{}, err
			}
			name = parsedName
			consumed = parsedConsumed
			labels = map[string]string{nameLabel: name}
		} else {
			_, parsedConsumed, err := parseOpenMetricsIdentifierRaw(line)
			if err != nil {
				return parsedSample{}, err
			}
			consumed = parsedConsumed
		}
		rest = line[consumed:]
		if len(rest) > 0 && rest[0] == '{' {
			end := findLabelSetEnd(rest)
			if end < 0 {
				return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: unterminated label set", line)
			}
			if _, err := parseOpenMetricsLabelsInto(labels, rest[1:end], false); err != nil {
				return parsedSample{}, err
			}
			rest = rest[end+1:]
		}
	}

	if len(rest) == 0 || rest[0] != ' ' {
		return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: missing value", line)
	}
	rest = trimOpenMetricsSpaces(rest)
	rawValue, rest, err := nextOpenMetricsToken(rest)
	if err != nil {
		return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: %w", line, err)
	}
	value, err := parseOpenMetricsFloat(rawValue)
	if err != nil {
		return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: invalid value: %w", line, err)
	}

	sample := parsedSample{
		Name:   name,
		Labels: labels,
		Value:  value,
	}
	rest = trimOpenMetricsSpaces(rest)
	if len(rest) == 0 {
		return sample, nil
	}
	if rest[0] == '#' {
		if err := parseOpenMetricsExemplar(rest); err != nil {
			return parsedSample{}, err
		}
		return sample, nil
	}

	rawTimestamp, timestampRest, err := nextOpenMetricsToken(rest)
	if err != nil {
		return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: %w", line, err)
	}
	timestamp, err := parseOpenMetricsTimestamp(rawTimestamp)
	if err != nil {
		return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: invalid timestamp: %w", line, err)
	}
	sample.Timestamp = timestamp
	timestampRest = trimOpenMetricsSpaces(timestampRest)
	if len(timestampRest) == 0 {
		return sample, nil
	}
	if timestampRest[0] != '#' {
		return parsedSample{}, fmt.Errorf("invalid OpenMetrics sample line %q: unexpected data after timestamp", line)
	}
	if err := parseOpenMetricsExemplar(timestampRest); err != nil {
		return parsedSample{}, err
	}
	return sample, nil
}

func parseOpenMetricsExemplar(data []byte) error {
	if len(data) < 2 || data[0] != '#' || data[1] != ' ' {
		return fmt.Errorf("invalid OpenMetrics exemplar %q", data)
	}
	rest := trimOpenMetricsSpaces(data[1:])
	if len(rest) == 0 || rest[0] != '{' {
		return fmt.Errorf("invalid OpenMetrics exemplar %q: missing labels", data)
	}
	end := findLabelSetEnd(rest)
	if end < 0 {
		return fmt.Errorf("invalid OpenMetrics exemplar %q: unterminated label set", data)
	}
	if _, err := parseOpenMetricsLabelsInto(nil, rest[1:end], false); err != nil {
		return err
	}
	rest = trimOpenMetricsSpaces(rest[end+1:])
	rawValue, rest, err := nextOpenMetricsToken(rest)
	if err != nil {
		return fmt.Errorf("invalid OpenMetrics exemplar %q: missing value", data)
	}
	if _, err := parseOpenMetricsFloat(rawValue); err != nil {
		return fmt.Errorf("invalid OpenMetrics exemplar %q: invalid value: %w", data, err)
	}
	rest = trimOpenMetricsSpaces(rest)
	if len(rest) == 0 {
		return nil
	}
	rawTimestamp, rest, err := nextOpenMetricsToken(rest)
	if err != nil {
		return fmt.Errorf("invalid OpenMetrics exemplar %q: invalid timestamp", data)
	}
	if _, err := parseOpenMetricsTimestamp(rawTimestamp); err != nil {
		return fmt.Errorf("invalid OpenMetrics exemplar %q: invalid timestamp: %w", data, err)
	}
	if len(trimOpenMetricsSpaces(rest)) > 0 {
		return fmt.Errorf("invalid OpenMetrics exemplar %q: unexpected trailing data", data)
	}
	return nil
}

func parseOpenMetricsLabelsInto(labels map[string]string, data []byte, allowMetricName bool) (string, error) {
	if labels == nil {
		return validateOpenMetricsLabels(data, allowMetricName)
	}

	metricName := ""
	var seen [8]string
	seenCount := 0
	var seenOverflow map[string]struct{}
	for len(data) > 0 {
		data = trimOpenMetricsSpaces(data)
		if len(data) == 0 {
			break
		}
		name, consumed, err := parseOpenMetricsIdentifier(data)
		if err != nil {
			return "", err
		}
		data = trimOpenMetricsSpaces(data[consumed:])
		if len(data) > 0 && data[0] == '=' {
			duplicate := false
			for i := 0; i < seenCount; i++ {
				if seen[i] == name {
					duplicate = true
					break
				}
			}
			if seenOverflow != nil {
				if _, ok := seenOverflow[name]; ok {
					duplicate = true
				}
			}
			if duplicate {
				return "", fmt.Errorf("invalid OpenMetrics label set %q: duplicate label name", data)
			}
			if seenCount < len(seen) {
				seen[seenCount] = name
				seenCount++
			} else {
				if seenOverflow == nil {
					seenOverflow = make(map[string]struct{}, 1)
				}
				seenOverflow[name] = struct{}{}
			}
			data = trimOpenMetricsSpaces(data[1:])
			if labels != nil {
				value, valueConsumed, err := parsePrometheusLabelValue(data)
				if err != nil {
					return "", err
				}
				labels[name] = value
				data = trimOpenMetricsSpaces(data[valueConsumed:])
			} else {
				valueConsumed, err := validatePrometheusLabelValue(data)
				if err != nil {
					return "", err
				}
				data = trimOpenMetricsSpaces(data[valueConsumed:])
			}
		} else {
			if !allowMetricName {
				return "", fmt.Errorf("invalid OpenMetrics label set %q: missing `=`", data)
			}
			if metricName != "" {
				return "", fmt.Errorf("invalid OpenMetrics label set %q: duplicate metric name", data)
			}
			metricName = name
		}

		if len(data) == 0 {
			break
		}
		if data[0] != ',' {
			return "", fmt.Errorf("invalid OpenMetrics label set %q: missing `,`", data)
		}
		data = data[1:]
	}
	return metricName, nil
}

func validateOpenMetricsLabels(data []byte, allowMetricName bool) (string, error) {
	metricName := ""
	var seen [8][]byte
	seenCount := 0
	var seenOverflow map[string]struct{}
	for len(data) > 0 {
		data = trimOpenMetricsSpaces(data)
		if len(data) == 0 {
			break
		}
		rawName, consumed, err := parseOpenMetricsIdentifierRaw(data)
		if err != nil {
			return "", err
		}
		data = trimOpenMetricsSpaces(data[consumed:])
		if len(data) > 0 && data[0] == '=' {
			duplicate := false
			for i := 0; i < seenCount; i++ {
				if bytes.Equal(seen[i], rawName) {
					duplicate = true
					break
				}
			}
			if seenOverflow != nil {
				if _, ok := seenOverflow[string(rawName)]; ok {
					duplicate = true
				}
			}
			if duplicate {
				return "", fmt.Errorf("invalid OpenMetrics label set %q: duplicate label name", data)
			}
			if seenCount < len(seen) {
				seen[seenCount] = rawName
				seenCount++
			} else {
				if seenOverflow == nil {
					seenOverflow = make(map[string]struct{}, 1)
				}
				seenOverflow[string(rawName)] = struct{}{}
			}
			data = trimOpenMetricsSpaces(data[1:])
			valueConsumed, err := validatePrometheusLabelValue(data)
			if err != nil {
				return "", err
			}
			data = trimOpenMetricsSpaces(data[valueConsumed:])
		} else {
			if !allowMetricName {
				return "", fmt.Errorf("invalid OpenMetrics label set %q: missing `=`", data)
			}
			if metricName != "" {
				return "", fmt.Errorf("invalid OpenMetrics label set %q: duplicate metric name", data)
			}
			var err error
			metricName, err = openMetricsIdentifierRawString(rawName)
			if err != nil {
				return "", err
			}
		}

		if len(data) == 0 {
			break
		}
		if data[0] != ',' {
			return "", fmt.Errorf("invalid OpenMetrics label set %q: missing `,`", data)
		}
		data = data[1:]
	}
	return metricName, nil
}

func parseOpenMetricsIdentifier(data []byte) (string, int, error) {
	raw, consumed, err := parseOpenMetricsIdentifierRaw(data)
	if err != nil {
		return "", 0, err
	}
	value, err := openMetricsIdentifierRawString(raw)
	if err != nil {
		return "", 0, err
	}
	return value, consumed, nil
}

func parseOpenMetricsIdentifierRaw(data []byte) ([]byte, int, error) {
	if len(data) == 0 {
		return nil, 0, errors.New("missing identifier")
	}
	if data[0] == '"' {
		consumed, err := validatePrometheusLabelValue(data)
		if err != nil {
			return nil, 0, err
		}
		return data[:consumed], consumed, nil
	}
	end := bytes.IndexAny(data, "{,}= ")
	if end < 0 {
		end = len(data)
	}
	if end == 0 {
		return nil, 0, fmt.Errorf("invalid OpenMetrics identifier %q", data)
	}
	if !validPrometheusMetricNameBytes(data[:end]) {
		return nil, 0, fmt.Errorf("invalid OpenMetrics identifier %q", data[:end])
	}
	return data[:end], end, nil
}

func openMetricsIdentifierRawString(raw []byte) (string, error) {
	if len(raw) > 0 && raw[0] == '"' {
		value, _, err := parsePrometheusLabelValue(raw)
		return value, err
	}
	return string(raw), nil
}

func parseOpenMetricsFloat(data []byte) (float64, error) {
	if !validPrometheusFloat(data) {
		return 0, fmt.Errorf("unsupported character in float %q", data)
	}
	return parseFloatAllowRange(data)
}

func parseOpenMetricsTimestamp(data []byte) (int64, error) {
	value, err := parseOpenMetricsFloat(data)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid timestamp %q", data)
	}
	return int64(value * 1000), nil
}

func nextOpenMetricsToken(data []byte) ([]byte, []byte, error) {
	if len(data) == 0 {
		return nil, nil, errors.New("missing token")
	}
	end := bytes.IndexByte(data, ' ')
	if end < 0 {
		return data, nil, nil
	}
	return data[:end], data[end:], nil
}

func trimOpenMetricsSpaces(data []byte) []byte {
	return bytes.TrimLeft(data, " ")
}

func unsafeString(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(data), len(data))
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
	return typ != ""
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
	value, err := parseFloatAllowRange(rawValue)
	if err != nil {
		return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: invalid value: %w", line, err)
	}
	sample := parsedSample{
		Name:   name,
		Labels: labels,
		Value:  value,
	}
	if len(rest) > 0 {
		if rest[0] == '#' {
			if err := parseOpenMetricsExemplar(rest); err != nil {
				return parsedSample{}, err
			}
			return sample, nil
		}
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
				if trailing[0] != '#' {
					return parsedSample{}, fmt.Errorf("invalid Prometheus sample line %q: unexpected data after timestamp", line)
				}
				if err := parseOpenMetricsExemplar(trailing); err != nil {
					return parsedSample{}, err
				}
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

func requiresPrometheusCompatibilityParser(data []byte) bool {
	return containsPrometheusCompatibilityMarker(data) ||
		bytes.ContainsAny(data, "\r\f\v") ||
		(len(data) > 0 && (data[0] == ' ' || data[0] == '\t'))
}

func containsExponent400(data []byte) bool {
	for offset := 0; offset < len(data); {
		index := bytes.Index(data[offset:], []byte("400"))
		if index < 0 {
			return false
		}
		start := offset + index
		if start > 0 && (data[start-1] == 'e' || data[start-1] == 'E') {
			return true
		}
		if start > 1 && (data[start-1] == '+' || data[start-1] == '-') && (data[start-2] == 'e' || data[start-2] == 'E') {
			return true
		}
		offset = start + 3
	}
	return false
}

func containsPrometheusCompatibilityMarker(data []byte) bool {
	for offset := 0; offset < len(data); {
		index := bytes.IndexByte(data[offset:], '#')
		if index < 0 {
			return false
		}
		start := offset + index
		if start > 0 && data[start-1] == ' ' && start+1 < len(data) && data[start+1] == ' ' {
			return true
		}
		if !bytes.HasPrefix(data[start:], []byte("# TYPE ")) {
			offset = start + 1
			continue
		}
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			end = len(data) - start
		}
		fields := bytes.Fields(data[start : start+end])
		if len(fields) == 4 && string(fields[0]) == "#" && string(fields[1]) == "TYPE" {
			typ := strings.ToLower(string(fields[3]))
			switch typ {
			case "counter", "gauge", "histogram", "summary", "untyped":
			default:
				return true
			}
		}
		offset = start + end + 1
	}
	return false
}

func sanitizeFloatOverflows(data []byte) []byte {
	if !possiblyContainsFloatOverflow(data) {
		return data
	}
	return sanitizeFloatOverflowsAll(data)
}

func sanitizeFloatOverflowsAll(data []byte) []byte {
	lines := bytes.Split(data, []byte{'\n'})
	changed := false
	for i, line := range lines {
		normalized := normalizeFloatOverflowLine(line)
		if !bytes.Equal(normalized, line) {
			lines[i] = normalized
			changed = true
		}
	}
	if !changed {
		return data
	}
	return bytes.Join(lines, []byte{'\n'})
}

func possiblyContainsFloatOverflow(data []byte) bool {
	return containsExponent400(data)
}

func normalizeFloatOverflowLine(line []byte) []byte {
	if len(line) == 0 || line[0] == '#' {
		return line
	}

	_, consumed, err := parseOpenMetricsIdentifierRaw(line)
	if err != nil {
		if line[0] != '{' {
			return line
		}
		consumed = 0
	}
	rest := line[consumed:]
	if len(rest) > 0 && rest[0] == '{' {
		end := findLabelSetEnd(rest)
		if end < 0 {
			return line
		}
		rest = rest[end+1:]
	}
	rest = bytes.TrimLeft(rest, " \t")
	if len(rest) == 0 {
		return line
	}
	valueStart := len(line) - len(rest)
	valueEnd := bytes.IndexAny(rest, " \t")
	if valueEnd < 0 {
		valueEnd = len(rest)
	}
	normalized := replaceFloatOverflow(line, valueStart, valueStart+valueEnd, true)
	rest = normalized[valueStart+valueEnd+len(normalized)-len(line):]
	rest = bytes.TrimLeft(rest, " \t")
	if len(rest) == 0 {
		return normalized
	}
	if rest[0] != '#' {
		_, rest, _ = nextOpenMetricsToken(rest)
		rest = bytes.TrimLeft(rest, " \t")
	}
	if len(rest) < 2 || rest[0] != '#' {
		return normalized
	}

	exemplar := bytes.TrimLeft(rest[1:], " \t")
	if len(exemplar) == 0 || exemplar[0] != '{' {
		return normalized
	}
	labelsEnd := findLabelSetEnd(exemplar)
	if labelsEnd < 0 {
		return normalized
	}
	exemplar = bytes.TrimLeft(exemplar[labelsEnd+1:], " \t")
	if len(exemplar) == 0 {
		return normalized
	}
	tokenEnd := bytes.IndexAny(exemplar, " \t")
	if tokenEnd < 0 {
		tokenEnd = len(exemplar)
	}
	tokenStart := len(normalized) - len(exemplar)
	return replaceFloatOverflow(normalized, tokenStart, tokenStart+tokenEnd, false)
}

func replaceFloatOverflow(line []byte, start, end int, preserveValue bool) []byte {
	value, err := strconv.ParseFloat(unsafeString(line[start:end]), 64)
	if !errors.Is(err, strconv.ErrRange) {
		return line
	}
	replacement := []byte("0")
	if preserveValue {
		replacement = []byte("+Inf")
		if value == 0 {
			replacement = []byte("0")
			if math.Signbit(value) {
				replacement = []byte("-0")
			}
		} else if math.Signbit(value) {
			replacement = []byte("-Inf")
		}
	}
	normalized := make([]byte, 0, len(line)-(end-start)+len(replacement))
	normalized = append(normalized, line[:start]...)
	normalized = append(normalized, replacement...)
	normalized = append(normalized, line[end:]...)
	return normalized
}

func parseFloatAllowRange(data []byte) (float64, error) {
	value, err := strconv.ParseFloat(unsafeString(data), 64)
	if errors.Is(err, strconv.ErrRange) {
		return value, nil
	}
	return value, err
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
