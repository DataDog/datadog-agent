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
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"

	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// jsonSample is the JSON-serializable representation of a single sample.
type jsonSample struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
	Value     float64           `json:"value"`
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

	// Find the last metric family boundary (a line starting with "# HELP " or "# TYPE ").
	// Only split there if there is at least one sample line before it.
	lines := strings.Split(state.buffer, "\n")
	lastBoundary := -1
	for i := len(lines) - 1; i > 0; i-- {
		if strings.HasPrefix(lines[i], "# HELP ") || strings.HasPrefix(lines[i], "# TYPE ") {
			// Verify there's sample data before this boundary.
			hasSample := false
			for j := 0; j < i; j++ {
				if lines[j] != "" && !strings.HasPrefix(lines[j], "#") {
					hasSample = true
					break
				}
			}
			if hasSample {
				lastBoundary = i
			}
			break
		}
	}

	if lastBoundary <= 0 {
		// No complete family boundary found; keep buffering.
		return "", nil
	}

	complete := strings.Join(lines[:lastBoundary], "\n")
	state.buffer = strings.Join(lines[lastBoundary:], "\n")

	return parseText(complete, state.contentType)
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
		parser = textparse.NewOpenMetricsParser(data, st, nil)
	default:
		parser = textparse.NewPromParser(data, st, false)
	}

	var families []jsonMetricFamily
	var lbls labels.Labels

	for {
		entry, err := parser.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Debugf("prometheus parser: skipping parse error: %v", err)
			break
		}

		switch entry {
		case textparse.EntryHelp:
			name, help := parser.Help()
			// Start or update family with help text
			if len(families) == 0 || families[len(families)-1].Name != string(name) {
				families = append(families, jsonMetricFamily{
					Name:    string(name),
					Help:    string(help),
					Samples: make([]jsonSample, 0, 8),
				})
			} else {
				families[len(families)-1].Help = string(help)
			}

		case textparse.EntryType:
			name, typ := parser.Type()
			sName := string(name)
			sType := strings.ToLower(string(typ))

			if len(families) > 0 && families[len(families)-1].Name == sName {
				families[len(families)-1].Type = sType
			} else {
				// Discard previous family if it has no samples
				if len(families) > 0 && len(families[len(families)-1].Samples) == 0 {
					families = families[:len(families)-1]
				}
				families = append(families, jsonMetricFamily{
					Name:    sName,
					Type:    sType,
					Samples: make([]jsonSample, 0, 8),
				})
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

			// Match to current family, trimming suffixes for histogram/summary
			if len(families) == 0 || !belongsToFamily(&families[len(families)-1], rawName) {
				// Create new untyped family
				if len(families) > 0 && len(families[len(families)-1].Samples) == 0 {
					families = families[:len(families)-1]
				}
				families = append(families, jsonMetricFamily{
					Name:    rawName,
					Type:    "untyped",
					Samples: make([]jsonSample, 0, 8),
				})
			}

			sample := jsonSample{
				Name:   rawName,
				Labels: labelMap,
				Value:  value,
			}
			if ts != nil {
				sample.Timestamp = ts
			}

			families[len(families)-1].Samples = append(families[len(families)-1].Samples, sample)
		}
	}

	// Discard last family if it has no samples
	if len(families) > 0 && len(families[len(families)-1].Samples) == 0 {
		families = families[:len(families)-1]
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

// belongsToFamily checks whether a sample name belongs to the given metric family,
// accounting for histogram/summary suffixes.
func belongsToFamily(family *jsonMetricFamily, sampleName string) bool {
	if family.Name == sampleName {
		return true
	}
	switch family.Type {
	case "histogram":
		for _, suffix := range []string{"_bucket", "_sum", "_count"} {
			if strings.TrimSuffix(sampleName, suffix) == family.Name {
				return true
			}
		}
	case "summary":
		for _, suffix := range []string{"_sum", "_count"} {
			if strings.TrimSuffix(sampleName, suffix) == family.Name {
				return true
			}
		}
	case "counter":
		if strings.TrimSuffix(sampleName, "_total") == family.Name {
			return true
		}
	}
	return false
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
