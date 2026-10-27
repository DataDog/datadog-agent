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
	"expvar"
	"fmt"
	"math"
	"net/http"
	"sort"
)

const remoteAgentsName = "remote_agents"

// Handler overlays remote values when /debug/vars is read, after local expvars
// have been evaluated. Other routes and installations without a registry are unchanged.
func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/vars" || expvar.Get(remoteAgentsName) == nil {
			next.ServeHTTP(w, r)
			return
		}

		capture := &responseCapture{header: make(http.Header)}
		next.ServeHTTP(capture, r)
		body := capture.body.Bytes()
		if capture.code == 0 || capture.code == http.StatusOK {
			if merged, err := MergeJSON(body); err == nil {
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

// MergeJSON combines only paths published under remote_agents with the local
// /debug/vars snapshot. The remote tree contains only explicitly mapped ADP
// equivalents; unrelated expvars remain untouched.
func MergeJSON(body []byte) ([]byte, error) {
	var snapshot map[string]any
	if err := decode(body, &snapshot); err != nil {
		return nil, err
	}
	merge(snapshot)
	result, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(result, '\n'), nil
}

// RootViews holds independent Core and per-agent remote values, as well as
// their combined view. No subtraction from a cached total is necessary.
// These maps are decoded for status templates; /debug/vars uses MergeJSON to
// preserve integer precision in the check's metrics.
type RootViews struct {
	Core          map[string]any
	RemoteByAgent map[string]map[string]any
	Combined      map[string]any
}

// ReadRoot takes one snapshot of the local expvar and the mapped RAR telemetry
// for a root. Status providers can render Core, ADP, and total independently.
func ReadRoot(name string) RootViews {
	views := RootViews{
		Core:          make(map[string]any),
		RemoteByAgent: make(map[string]map[string]any),
		Combined:      make(map[string]any),
	}
	snapshot := make(map[string]any)
	if local := expvar.Get(name); local != nil {
		var root map[string]any
		if err := decode([]byte(local.String()), &root); err == nil && root != nil {
			snapshot[name] = root
			views.Core = statusView(root)
		}
	}
	if remote := expvar.Get(remoteAgentsName); remote != nil {
		var agents map[string]any
		if err := decode([]byte(remote.String()), &agents); err == nil {
			snapshot[remoteAgentsName] = agents
			for agentName, raw := range agents {
				roots, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if root, ok := roots[name].(map[string]any); ok {
					views.RemoteByAgent[agentName] = statusView(root)
				}
			}
		}
	}
	merge(snapshot)
	if root, ok := snapshot[name].(map[string]any); ok {
		views.Combined = statusView(root)
	}
	return views
}

func statusView(root map[string]any) map[string]any {
	// Status templates expect the float64 numbers produced by json.Unmarshal;
	// json.Number("0") would be truthy in a template and display empty counters.
	encoded, err := json.Marshal(root)
	if err != nil {
		return make(map[string]any)
	}
	var view map[string]any
	if err := json.Unmarshal(encoded, &view); err != nil {
		return make(map[string]any)
	}
	return view
}

func decode(body []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode expvar snapshot: %w", err)
	}
	return nil
}

func merge(snapshot map[string]any) {
	agents, ok := snapshot[remoteAgentsName].(map[string]any)
	if !ok {
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
			if rootName == remoteAgentsName {
				continue
			}
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
