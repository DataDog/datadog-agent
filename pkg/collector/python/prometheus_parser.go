// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

/*
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
)

// parserState holds the state for an incremental prometheus parser.
type parserState struct {
	contentType string
	buffer      string // accumulated but not yet parsed text
}

var (
	parserRegistry  sync.Map
	parserIDCounter atomic.Int64
)

// jsonMetricFamily is the JSON-serializable representation of a parsed metric family.
type jsonMetricFamily struct {
	Name    string       `json:"name"`
	Type    string       `json:"type"`
	Help    string       `json:"help,omitempty"`
	Samples []jsonSample `json:"samples"`
}

// jsonFloat is a float64 that serializes NaN and ±Inf as JSON strings so that
// encoding/json does not reject them. The Python caller decodes these strings
// back to the corresponding float values.
type jsonFloat float64

func (f jsonFloat) MarshalJSON() ([]byte, error) {
	v := float64(f)
	switch {
	case math.IsNaN(v):
		return []byte(`"NaN"`), nil
	case math.IsInf(v, 1):
		return []byte(`"+Inf"`), nil
	case math.IsInf(v, -1):
		return []byte(`"-Inf"`), nil
	default:
		return json.Marshal(v)
	}
}

// jsonSample is the JSON-serializable representation of a single sample.
type jsonSample struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Value     jsonFloat         `json:"value"`
	Timestamp *int64            `json:"timestamp,omitempty"`
}

// newPrometheusParser creates a new incremental parser and returns its handle.
func newPrometheusParser(contentType string) int64 {
	id := parserIDCounter.Add(1)
	parserRegistry.Store(id, &parserState{
		contentType: contentType,
	})
	return id
}

// feedPrometheusParser feeds a chunk of lines to the parser and returns JSON
// for any complete metric families. Incomplete trailing data is buffered.
func feedPrometheusParser(id int64, chunk string) (string, error) {
	val, ok := parserRegistry.Load(id)
	if !ok {
		return "", errors.New("unknown parser id")
	}
	state := val.(*parserState)

	if state.buffer != "" {
		state.buffer += "\n" + chunk
	} else {
		state.buffer = chunk
	}

	lines := strings.Split(state.buffer, "\n")
	lastBoundary := -1

	// Scan backward for the last HELP/TYPE line, then walk back to the start of
	// its contiguous metadata block so that HELP and TYPE lines for the same
	// family are never split across the boundary. Only split if at least one
	// sample line precedes the block (i.e. a complete family exists before it).
	for i := len(lines) - 1; i > 0; i-- {
		if strings.HasPrefix(lines[i], "# HELP ") || strings.HasPrefix(lines[i], "# TYPE ") {
			start := i
			for start > 0 && (strings.HasPrefix(lines[start-1], "# HELP ") || strings.HasPrefix(lines[start-1], "# TYPE ")) {
				start--
			}
			for j := 0; j < start; j++ {
				if lines[j] != "" && !strings.HasPrefix(lines[j], "#") {
					lastBoundary = start
					break
				}
			}
			break
		}
	}

	// Fallback for sample-only expositions (no HELP/TYPE directives): scan only
	// the tail of the buffer (the most recently appended chunk) for a metric-name
	// transition. This bounds the scan to O(chunk size) per call rather than
	// O(buffer size), avoiding quadratic growth on large exporters.
	if lastBoundary <= 0 {
		chunkLines := strings.Count(chunk, "\n") + 2
		scanFrom := len(lines) - chunkLines
		if scanFrom < 0 {
			scanFrom = 0
		}
		lastName := ""
		for i := len(lines) - 1; i >= scanFrom; i-- {
			line := lines[i]
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			name := sampleMetricName(line)
			if lastName == "" {
				lastName = name
				continue
			}
			if name != lastName {
				lastBoundary = i + 1
				break
			}
		}
	}

	if lastBoundary <= 0 {
		return "", nil
	}

	complete := strings.Join(lines[:lastBoundary], "\n")
	state.buffer = strings.Join(lines[lastBoundary:], "\n")

	return parseText(complete, state.contentType)
}

// sampleMetricName extracts the metric name from a Prometheus sample line.
func sampleMetricName(line string) string {
	end := strings.IndexAny(line, "{ \t")
	if end < 0 {
		return line
	}
	return line[:end]
}

// finishPrometheusParser parses any remaining buffered data and removes the parser.
func finishPrometheusParser(id int64) (string, error) {
	val, ok := parserRegistry.LoadAndDelete(id)
	if !ok {
		return "", errors.New("unknown parser id")
	}
	state := val.(*parserState)

	buf := strings.TrimSpace(state.buffer)
	if buf == "" {
		return "", nil
	}

	return parseText(state.buffer, state.contentType)
}

// parseText parses a complete block of prometheus/openmetrics text and returns
// the metric families as a JSON string.
//
// Family grouping follows the structure of the text itself: a # HELP or # TYPE
// directive opens a new family, and all subsequent series belong to that family
// until the next directive. This mirrors the Python prometheus_client approach
// and requires no knowledge of type-specific metric name suffixes, making it
// correct for all current and future OpenMetrics types.
//
// For sample-only expositions (no directives), samples are grouped by exact
// metric name — a new family starts whenever the name changes.
func parseText(text string, contentType string) (string, error) {
	data := []byte(text)

	st := labels.NewSymbolTable()
	var parser textparse.Parser
	mediaType := contentType
	if idx := strings.IndexByte(mediaType, ';'); idx >= 0 {
		mediaType = mediaType[:idx]
	}
	switch mediaType {
	case "application/openmetrics-text":
		// Intermediate chunks from feedPrometheusParser won't have # EOF.
		// The OpenMetrics parser requires it, so append it when missing.
		trimmed := strings.TrimRight(text, " \t\n\r")
		if !strings.HasSuffix(trimmed, "# EOF") {
			data = append(data, "\n# EOF\n"...)
		}
		parser = textparse.NewOpenMetricsParser(data, st)
	default:
		parser = textparse.NewPromParser(data, st, false)
	}

	var families []jsonMetricFamily
	currentIdx := -1 // index into families of the active family; -1 = none yet
	var lbls labels.Labels

	for {
		entry, err := parser.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}

		switch entry {
		case textparse.EntryHelp:
			name, help := parser.Help()
			sName := string(name)
			if currentIdx >= 0 && families[currentIdx].Name == sName {
				families[currentIdx].Help = string(help)
			} else {
				if currentIdx >= 0 && len(families[currentIdx].Samples) == 0 {
					families = families[:currentIdx]
				}
				families = append(families, jsonMetricFamily{
					Name:    sName,
					Help:    string(help),
					Samples: make([]jsonSample, 0, 8),
				})
				currentIdx = len(families) - 1
			}

		case textparse.EntryType:
			name, typ := parser.Type()
			sName := string(name)
			sType := strings.ToLower(string(typ))
			// Preserve the TYPE-line name verbatim (e.g. "foo_total" stays
			// "foo_total").  The Python _json_to_metric function handles
			// stripping _total for standard counters and adding it for
			// non-standard ones (# TYPE foo counter with sample foo_total).
			// Stripping here would make the two patterns indistinguishable.
			if currentIdx >= 0 && families[currentIdx].Name == sName {
				// Update the family opened by EntryHelp.
				families[currentIdx].Type = sType
			} else {
				if currentIdx >= 0 && len(families[currentIdx].Samples) == 0 {
					families = families[:currentIdx]
				}
				families = append(families, jsonMetricFamily{
					Name:    sName,
					Type:    sType,
					Samples: make([]jsonSample, 0, 8),
				})
				currentIdx = len(families) - 1
			}

		case textparse.EntrySeries:
			_, ts, value := parser.Series()
			parser.Labels(&lbls)

			rawName := ""
			labelMap := make(map[string]string, lbls.Len())
			lbls.Range(func(l labels.Label) {
				if l.Name == "__name__" {
					rawName = l.Value
				} else {
					labelMap[l.Name] = l.Value
				}
			})

			// For a typed family (opened by # HELP / # TYPE), all series until
			// the next directive belong to it — no suffix detection needed.
			// For an untyped family (sample-only exposition), group by exact
			// metric name; start a new family when the name changes.
			if currentIdx < 0 || (families[currentIdx].Type == "untyped" && families[currentIdx].Name != rawName) {
				if currentIdx >= 0 && len(families[currentIdx].Samples) == 0 {
					families = families[:currentIdx]
				}
				families = append(families, jsonMetricFamily{
					Name:    rawName,
					Type:    "untyped",
					Samples: make([]jsonSample, 0, 8),
				})
				currentIdx = len(families) - 1
			}

			sample := jsonSample{
				Name:   rawName,
				Labels: labelMap,
				Value:  jsonFloat(value),
			}
			if ts != nil {
				sample.Timestamp = ts
			}
			families[currentIdx].Samples = append(families[currentIdx].Samples, sample)
		}
	}

	// Discard trailing empty family (e.g. a # HELP/TYPE with no following samples).
	if currentIdx >= 0 && len(families[currentIdx].Samples) == 0 {
		families = families[:currentIdx]
	}

	if len(families) == 0 {
		return "", nil
	}

	result, err := json.Marshal(families)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// --- CGo exported functions ---

//export NewPrometheusParser
func NewPrometheusParser(contentType *C.char) C.long {
	goContentType := C.GoString(contentType)
	id := newPrometheusParser(goContentType)
	return C.long(id)
}

//export FeedPrometheusParser
func FeedPrometheusParser(parserID C.long, chunk *C.char, result **C.char, errOut **C.char) {
	*result = nil
	*errOut = nil

	goChunk := C.GoString(chunk)
	jsonStr, err := feedPrometheusParser(int64(parserID), goChunk)
	if err != nil {
		*errOut = TrackedCString(err.Error())
		return
	}
	if jsonStr != "" {
		*result = TrackedCString(jsonStr)
	}
}

//export FinishPrometheusParser
func FinishPrometheusParser(parserID C.long, result **C.char, errOut **C.char) {
	*result = nil
	*errOut = nil

	jsonStr, err := finishPrometheusParser(int64(parserID))
	if err != nil {
		*errOut = TrackedCString(err.Error())
		return
	}
	if jsonStr != "" {
		*result = TrackedCString(jsonStr)
	}
}
