// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"github.com/DataDog/datadog-agent/test/localdog/store"
)

// intakeHandler handles every request that is not part of the localdog query API:
// it is what the Datadog Agent talks to when DD_DD_URL points at localdog.
func (s *Server) intakeHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if s.serveUI(w, r) {
		return
	}
	switch {
	case path == "/api/v1/validate":
		writeJSON(w, http.StatusOK, map[string]any{"valid": true})
		return
	case strings.HasPrefix(path, "/api/v0.1/configurations"), strings.HasPrefix(path, "/api/v0.1/org"), strings.HasPrefix(path, "/api/v0.1/status"):
		// Remote Config is not supported: tell the agent so it backs off quietly.
		http.Error(w, "remote config not supported by localdog", http.StatusNotFound)
		return
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	encoding := r.Header.Get("Content-Encoding")
	payload := api.Payload{
		Timestamp:   time.Now(),
		APIKey:      r.Header.Get("Dd-Api-Key"),
		Data:        body,
		Encoding:    encoding,
		ContentType: r.Header.Get("Content-Type"),
	}
	s.store.RecordPayload(path)

	var perr error
	switch {
	case path == "/api/v2/series":
		perr = s.ingestSeries(aggregator.ParseMetricSeries, payload)
	case path == "/api/intake/metrics/v3/series":
		perr = s.ingestSeries(aggregator.ParseMetricSeriesV3, payload)
	case path == "/api/v1/series":
		perr = s.ingestSeries(aggregator.ParseMetricSeriesV1, payload)
	case path == "/api/beta/sketches":
		perr = s.ingestSketches(payload)
	case path == "/api/v1/check_run":
		perr = s.ingestCheckRuns(payload)
	case path == "/api/v2/logs" || path == "/v1/input" || strings.HasPrefix(path, "/v1/input/"):
		perr = s.ingestLogs(payload)
	case path == "/api/v0.2/traces":
		perr = s.ingestTraces(payload)
	case path == "/api/v0.2/stats":
		perr = s.ingestAPMStats(payload)
	default:
		// Everything else (metadata, processes, stats, orchestrator, telemetry...) is accepted and dropped.
	}
	if perr != nil {
		log.Printf("localdog: failed to parse payload on %s (encoding=%q content-type=%q): %v", path, encoding, payload.ContentType, perr)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{}`))
}

var metricTypes = map[int32]string{0: "unspecified", 1: "count", 2: "rate", 3: "gauge"}

func (s *Server) ingestSeries(parse func(api.Payload) ([]*aggregator.MetricSeries, error), payload api.Payload) error {
	series, err := parse(payload)
	if err != nil {
		return err
	}
	for _, ser := range series {
		host := ""
		for _, res := range ser.Resources {
			if res.Type == "host" {
				host = res.Name
			}
		}
		points := make([]store.Point, 0, len(ser.Points))
		for _, p := range ser.Points {
			points = append(points, store.Point{Timestamp: p.Timestamp, Value: p.Value})
		}
		s.store.AddPoints(ser.Metric, metricTypes[int32(ser.Type)], host, ser.Unit, ser.Interval, ser.Tags, points)
	}
	return nil
}

func (s *Server) ingestSketches(payload api.Payload) error {
	sketches, err := aggregator.ParseSketches(payload)
	if err != nil {
		return err
	}
	for _, sk := range sketches {
		var avg, count, sum, minV, maxV []store.Point
		for _, d := range sk.Dogsketches {
			if d.Cnt == 0 {
				continue
			}
			count = append(count, store.Point{Timestamp: d.Ts, Value: float64(d.Cnt)})
			sum = append(sum, store.Point{Timestamp: d.Ts, Value: d.Sum})
			avg = append(avg, store.Point{Timestamp: d.Ts, Value: d.Avg})
			minV = append(minV, store.Point{Timestamp: d.Ts, Value: d.Min})
			maxV = append(maxV, store.Point{Timestamp: d.Ts, Value: d.Max})
		}
		// Distributions are exposed through their usual aggregates so they can be graphed like any metric.
		s.store.AddPoints(sk.Metric, "distribution", sk.Host, "", 10, sk.Tags, avg)
		s.store.AddPoints(sk.Metric+".count", "count", sk.Host, "", 10, sk.Tags, count)
		s.store.AddPoints(sk.Metric+".sum", "count", sk.Host, "", 10, sk.Tags, sum)
		s.store.AddPoints(sk.Metric+".min", "gauge", sk.Host, "", 10, sk.Tags, minV)
		s.store.AddPoints(sk.Metric+".max", "gauge", sk.Host, "", 10, sk.Tags, maxV)
	}
	return nil
}

func (s *Server) ingestCheckRuns(payload api.Payload) error {
	checks, err := aggregator.ParseCheckRunPayload(payload)
	if err != nil {
		return err
	}
	for _, c := range checks {
		// Service checks become a gauge of their status (0=OK,1=WARN,2=CRITICAL,3=UNKNOWN).
		s.store.AddPoints("check_run."+c.Check, "gauge", c.HostName, "", 0, c.Tags, []store.Point{{Timestamp: int64(c.Timestamp), Value: float64(c.Status)}})
	}
	return nil
}

// reservedLogKeys are the top-level keys of an intake log that are not custom attributes.
var reservedLogKeys = map[string]bool{
	"message": true, "status": true, "timestamp": true, "hostname": true,
	"service": true, "ddsource": true, "ddtags": true,
}

func (s *Server) ingestLogs(payload api.Payload) error {
	if len(payload.Data) == 0 {
		return nil
	}
	inflated, err := aggregator.Inflate(payload.Data, payload.Encoding)
	if err != nil {
		return err
	}
	var raw []map[string]any
	if err := json.Unmarshal(inflated, &raw); err != nil {
		var single map[string]any
		if err2 := json.Unmarshal(inflated, &single); err2 != nil {
			return err
		}
		raw = []map[string]any{single}
	}
	logs := make([]*store.Log, 0, len(raw))
	for _, entry := range raw {
		if len(entry) == 0 {
			continue
		}
		logs = append(logs, normalizeLog(entry))
	}
	s.store.AddLogs(logs)
	return nil
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(t)
	}
}

// normalizeLog turns an intake log entry into a stored log. When the message itself is a JSON
// object (e.g. a structured app log tailed from a file) its fields are promoted to attributes,
// mirroring the Datadog JSON preprocessing.
func normalizeLog(entry map[string]any) *store.Log {
	l := &store.Log{
		Message:    asString(entry["message"]),
		Status:     strings.ToLower(asString(entry["status"])),
		Host:       asString(entry["hostname"]),
		Service:    asString(entry["service"]),
		Source:     asString(entry["ddsource"]),
		Attributes: map[string]any{},
	}
	if tags := asString(entry["ddtags"]); tags != "" {
		l.Tags = strings.Split(tags, ",")
	}
	switch ts := entry["timestamp"].(type) {
	case float64:
		l.Timestamp = int64(ts)
	case string:
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			l.Timestamp = t.UnixMilli()
		}
	}
	for k, v := range entry {
		if !reservedLogKeys[k] {
			l.Attributes[k] = v
		}
	}

	trimmed := strings.TrimSpace(l.Message)
	if strings.HasPrefix(trimmed, "{") {
		var structured map[string]any
		if err := json.Unmarshal([]byte(trimmed), &structured); err == nil {
			for k, v := range structured {
				l.Attributes[k] = v
			}
			l.Message = firstString(structured, "message", "msg")
			if l.Message == "" {
				l.Message = trimmed
			}
			if lvl := firstString(structured, "level", "status", "severity", "log.level"); lvl != "" {
				l.Status = strings.ToLower(lvl)
			}
			if svc := firstString(structured, "service", "dd.service"); svc != "" && l.Service == "" {
				l.Service = svc
			}
			if ts := firstString(structured, "timestamp", "time", "@timestamp"); ts != "" {
				if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
					l.Timestamp = t.UnixMilli()
				}
			}
		}
	}
	l.Attributes = nestAttributes(l.Attributes)
	l.Status = normalizeStatus(l.Status)
	if l.Timestamp == 0 {
		l.Timestamp = time.Now().UnixMilli()
	}

	// Trace correlation: dd-trace log injection writes dd.trace_id / dd.span_id.
	if dd, ok := l.Attributes["dd"].(map[string]any); ok {
		l.TraceID = asString(dd["trace_id"])
		l.SpanID = asString(dd["span_id"])
		if l.Service == "" {
			l.Service = asString(dd["service"])
		}
	}
	if l.TraceID == "" {
		l.TraceID = firstString(l.Attributes, "dd.trace_id", "trace_id")
	}
	if l.SpanID == "" {
		l.SpanID = firstString(l.Attributes, "dd.span_id", "span_id")
	}
	l.TraceID = normalizeTraceID(l.TraceID)
	return l
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := asString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

// normalizeStatus maps the many severity spellings onto the Datadog log statuses.
func normalizeStatus(s string) string {
	switch strings.ToLower(s) {
	case "emerg", "emergency", "f", "fatal", "panic":
		return "emergency"
	case "a", "alert":
		return "alert"
	case "c", "crit", "critical":
		return "critical"
	case "e", "err", "error":
		return "error"
	case "w", "warn", "warning":
		return "warn"
	case "n", "notice":
		return "notice"
	case "i", "info", "information", "informational":
		return "info"
	case "d", "debug", "trace", "verbose":
		return "debug"
	case "ok", "success":
		return "ok"
	case "":
		return "info"
	}
	return strings.ToLower(s)
}

func (s *Server) ingestTraces(payload api.Payload) error {
	parsed, err := aggregator.ParseTracePayload(payload)
	if err != nil {
		return err
	}
	var spans []*store.Span
	for _, ap := range parsed {
		for _, tp := range ap.TracerPayloads {
			spans = append(spans, convertTracerPayload(ap.AgentPayload, tp)...)
		}
		for _, itp := range ap.IdxTracerPayloads {
			spans = append(spans, convertIdxTracerPayload(ap.AgentPayload, itp)...)
		}
	}
	s.store.AddSpans(spans)
	return nil
}

func convertTracerPayload(ap *pb.AgentPayload, tp *pb.TracerPayload) []*store.Span {
	env := firstNonEmpty(tp.Env, ap.Env)
	host := firstNonEmpty(tp.Hostname, ap.HostName)
	var out []*store.Span
	for _, chunk := range tp.Chunks {
		tidHigh := chunk.Tags["_dd.p.tid"]
		inChunk := map[uint64]bool{}
		for _, sp := range chunk.Spans {
			inChunk[sp.SpanID] = true
		}
		for _, sp := range chunk.Spans {
			if tidHigh == "" {
				tidHigh = sp.Meta["_dd.p.tid"]
			}
			meta := sp.Meta
			if meta == nil {
				meta = map[string]string{}
			}
			ss := &store.Span{
				TraceID:     strconv.FormatUint(sp.TraceID, 10),
				SpanID:      strconv.FormatUint(sp.SpanID, 10),
				ParentID:    strconv.FormatUint(sp.ParentID, 10),
				Service:     sp.Service,
				Name:        sp.Name,
				Resource:    sp.Resource,
				Type:        sp.Type,
				Env:         firstNonEmpty(meta["env"], env),
				Host:        host,
				Version:     firstNonEmpty(meta["version"], tp.AppVersion),
				Start:       sp.Start,
				Duration:    sp.Duration,
				Error:       sp.Error,
				Meta:        meta,
				Metrics:     sp.Metrics,
				IsRoot:      sp.ParentID == 0,
				TraceIDHigh: tidHigh,
			}
			if ss.Metrics == nil {
				ss.Metrics = map[string]float64{}
			}
			if tp.LanguageName != "" {
				meta["language"] = firstNonEmpty(meta["language"], tp.LanguageName)
			}
			out = append(out, ss)
		}
	}
	markTopLevel(out)
	return out
}

// markTopLevel flags service entry spans like the trace agent does: spans whose parent is not in
// the payload, or belongs to another service.
func markTopLevel(spans []*store.Span) {
	service := make(map[string]string, len(spans))
	for _, sp := range spans {
		service[sp.TraceID+"/"+sp.SpanID] = sp.Service
	}
	for _, sp := range spans {
		parentService, ok := service[sp.TraceID+"/"+sp.ParentID]
		sp.IsTopLevel = sp.ParentID == "0" || !ok || parentService != sp.Service
	}
}

func convertIdxTracerPayload(ap *pb.AgentPayload, ptp *idx.TracerPayload) []*store.Span {
	tp := idx.FromProto(ptp)
	env := firstNonEmpty(tp.Env(), ap.Env)
	host := firstNonEmpty(tp.Hostname(), ap.HostName)
	var out []*store.Span
	for _, chunk := range tp.Chunks {
		traceID := strconv.FormatUint(chunk.LegacyTraceID(), 10)
		tidHigh := ""
		if h := chunk.TraceIDHigh(); h != 0 {
			tidHigh = strconv.FormatUint(h, 16)
		}
		for _, sp := range chunk.Spans {
			meta := map[string]string{}
			metrics := map[string]float64{}
			for k, v := range sp.Attributes() {
				key := tp.Strings.Get(k)
				switch val := v.Value.(type) {
				case *idx.AnyValue_DoubleValue:
					metrics[key] = val.DoubleValue
				case *idx.AnyValue_IntValue:
					metrics[key] = float64(val.IntValue)
				default:
					meta[key] = v.AsString(tp.Strings)
				}
			}
			if k := sp.SpanKind(); k != "" {
				meta["span.kind"] = k
			}
			if c := sp.Component(); c != "" {
				meta["component"] = c
			}
			var errFlag int32
			if sp.Error() {
				errFlag = 1
			}
			out = append(out, &store.Span{
				TraceID:     traceID,
				TraceIDHigh: tidHigh,
				SpanID:      strconv.FormatUint(sp.SpanID(), 10),
				ParentID:    strconv.FormatUint(sp.ParentID(), 10),
				Service:     sp.Service(),
				Name:        sp.Name(),
				Resource:    sp.Resource(),
				Type:        sp.Type(),
				Env:         firstNonEmpty(sp.Env(), env),
				Host:        host,
				Version:     firstNonEmpty(sp.Version(), tp.AppVersion()),
				Start:       int64(sp.Start()),
				Duration:    int64(sp.Duration()),
				Error:       errFlag,
				Meta:        meta,
				Metrics:     metrics,
				IsRoot:      sp.ParentID() == 0,
			})
		}
	}
	markTopLevel(out)
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// normalizeTraceID converts 128-bit hex trace IDs (as written by dd-trace log injection) to the
// decimal low 64 bits used by spans, so logs and traces correlate.
func normalizeTraceID(id string) string {
	if id == "" {
		return id
	}
	if _, err := strconv.ParseUint(id, 10, 64); err == nil {
		return id
	}
	if len(id) == 32 {
		if low, err := strconv.ParseUint(id[16:], 16, 64); err == nil {
			return strconv.FormatUint(low, 10)
		}
	}
	return id
}

// nestAttributes expands dotted keys ("http.status_code") into nested objects, as Datadog does for
// JSON log attributes, so `@http.status_code` and the `http` object resolve the same way.
func nestAttributes(attrs map[string]any) map[string]any {
	out := map[string]any{}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	// shorter keys first so objects exist before their dotted children are merged in
	sort.Strings(keys)
	for _, k := range keys {
		v := attrs[k]
		if m, ok := v.(map[string]any); ok {
			v = nestAttributes(m)
		}
		if !strings.Contains(k, ".") {
			if existing, ok := out[k].(map[string]any); ok {
				if m, ok := v.(map[string]any); ok {
					for mk, mv := range m {
						existing[mk] = mv
					}
					continue
				}
			}
			out[k] = v
			continue
		}
		nest(out, k, v)
	}
	return out
}

// ingestAPMStats turns the APM stats computed by the agent into the trace metrics Datadog derives
// from them: trace.<span name>.hits / .errors (counts), .duration (total seconds) and
// trace.<span name> (average latency in seconds).
func (s *Server) ingestAPMStats(payload api.Payload) error {
	parsed, err := aggregator.ParseAPMStatsPayload(payload)
	if err != nil {
		return err
	}
	for _, p := range parsed {
		for _, csp := range p.Stats {
			host := firstNonEmpty(csp.Hostname, p.AgentHostname)
			env := firstNonEmpty(csp.Env, p.AgentEnv)
			for _, bucket := range csp.Stats {
				ts := int64(bucket.Start / 1e9)
				interval := int64(bucket.Duration / 1e9)
				for _, g := range bucket.Stats {
					if g.Name == "" {
						continue
					}
					tags := []string{"env:" + env, "service:" + g.Service, "resource_name:" + g.Resource}
					if csp.Version != "" {
						tags = append(tags, "version:"+csp.Version)
					}
					if g.HTTPStatusCode != 0 {
						tags = append(tags, "http.status_code:"+strconv.FormatUint(uint64(g.HTTPStatusCode), 10))
					}
					if g.SpanKind != "" {
						tags = append(tags, "span.kind:"+g.SpanKind)
					}
					tags = append(tags, g.PeerTags...)
					name := "trace." + g.Name
					point := func(v float64) []store.Point { return []store.Point{{Timestamp: ts, Value: v}} }
					s.store.AddPoints(name+".hits", "count", host, "hit", interval, tags, point(float64(g.Hits)))
					s.store.AddPoints(name+".errors", "count", host, "error", interval, tags, point(float64(g.Errors)))
					s.store.AddPoints(name+".duration", "gauge", host, "second", interval, tags, point(float64(g.Duration)/1e9))
					if g.Hits > 0 {
						s.store.AddPoints(name, "distribution", host, "second", interval, tags, point(float64(g.Duration)/1e9/float64(g.Hits)))
					}
				}
			}
		}
	}
	return nil
}
