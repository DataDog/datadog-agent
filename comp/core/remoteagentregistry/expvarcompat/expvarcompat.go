// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package expvarcompat projects mapped remote-agent values onto legacy expvar snapshots.
// It does not modify the process-wide expvar registry or its local counters.
package expvarcompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
)

// Source returns a fresh snapshot of mapped remote-agent values, grouped by agent.
// The registry protects its underlying counter cache; consumers must not modify the snapshot.
type Source func() map[string]any

// Handler overlays remote values when /debug/vars is read, after local expvars
// have been evaluated. Other routes and installations without a registry are unchanged.
func Handler(next http.Handler, source Source) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/vars" || source == nil {
			next.ServeHTTP(w, r)
			return
		}

		capture := &responseCapture{header: make(http.Header)}
		next.ServeHTTP(capture, r)
		body := capture.body.Bytes()
		if capture.code == 0 || capture.code == http.StatusOK {
			if merged, err := MergeJSON(body, source()); err == nil {
				body = merged
			}
		}
		for key, values := range capture.header {
			w.Header()[key] = append([]string(nil), values...)
		}
		if capture.code != 0 {
			w.WriteHeader(capture.code)
		}
		_, _ = w.Write(body)
	})
}

type responseCapture struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (c *responseCapture) Header() http.Header         { return c.header }
func (c *responseCapture) WriteHeader(code int)        { c.code = code }
func (c *responseCapture) Write(b []byte) (int, error) { return c.body.Write(b) }

// MergeJSON combines the mapped ADP snapshot with the local /debug/vars body.
// Unrelated expvars remain untouched; remote values are not published separately.
func MergeJSON(body []byte, remote map[string]any) ([]byte, error) {
	var snapshot map[string]any
	if err := decode(body, &snapshot); err != nil {
		return nil, err
	}
	merge(snapshot, remote)
	result, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(result, '\n'), nil
}

func decode(body []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode expvar snapshot: %w", err)
	}
	return nil
}

func merge(snapshot, agents map[string]any) {
	if len(agents) == 0 {
		return
	}
	// Saturation must not depend on Go map iteration order.
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		roots, ok := agents[name].(map[string]any)
		if !ok {
			continue
		}
		for rootName, remote := range roots {
			remoteMap, ok := remote.(map[string]any)
			if !ok {
				continue
			}
			local, exists := snapshot[rootName]
			if !exists {
				local = make(map[string]any)
				snapshot[rootName] = local
			}
			localMap, ok := local.(map[string]any)
			if ok {
				mergeMap(localMap, remoteMap)
			}
		}
	}
}

func mergeMap(local, remote map[string]any) {
	for key, remoteValue := range remote {
		if nested, ok := remoteValue.(map[string]any); ok {
			current, exists := local[key]
			if !exists {
				current = make(map[string]any)
				local[key] = current
			}
			if child, ok := current.(map[string]any); ok {
				mergeMap(child, nested)
			}
			continue
		}
		remoteNumber, ok := number(remoteValue)
		if !ok {
			continue
		}
		current, exists := local[key]
		if !exists {
			local[key] = remoteNumber
		} else if localNumber, ok := number(current); ok {
			local[key] = saturatingAdd(localNumber, remoteNumber)
		}
	}
}

func number(value any) (int64, bool) {
	switch n := value.(type) {
	case json.Number:
		value, err := n.Int64()
		return value, err == nil
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

func saturatingAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	if b < 0 && a < math.MinInt64-b {
		return math.MinInt64
	}
	return a + b
}
