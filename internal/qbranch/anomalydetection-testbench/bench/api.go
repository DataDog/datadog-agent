// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"context"
	"encoding/json"
	"fmt"
	stdlog "log"
	"math"
	"net/http"
	"net/http/pprof"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	testbenchimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/impl-testbench"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

// BenchAPI handles HTTP API requests for the bench.
type BenchAPI struct {
	tb     *Bench
	server *http.Server
}

// NewBenchAPI creates a new API handler.
func NewBenchAPI(tb *Bench) *BenchAPI {
	return &BenchAPI{tb: tb}
}

// Start starts the HTTP server.
func (api *BenchAPI) Start(addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/events", api.handleSSE)
	mux.HandleFunc("/api/progress", api.cors(api.handleProgress))
	mux.HandleFunc("/api/status", api.cors(api.handleStatus))
	mux.HandleFunc("/api/scenarios", api.cors(api.handleScenarios))
	mux.HandleFunc("/api/scenarios/", api.cors(api.handleScenarioAction))
	mux.HandleFunc("/api/components", api.cors(api.handleComponents))
	mux.HandleFunc("/api/series", api.cors(api.handleSeriesList))
	mux.HandleFunc("/api/series/id/", api.cors(api.handleSeriesDataByID))
	mux.HandleFunc("/api/series/", api.cors(api.handleSeriesData))
	mux.HandleFunc("/api/anomalies", api.cors(api.handleAnomalies))
	mux.HandleFunc("/api/logs/summary", api.cors(api.handleLogsSummary))
	mux.HandleFunc("/api/logs", api.cors(api.handleLogs))
	mux.HandleFunc("/api/log-anomalies", api.cors(api.handleLogAnomalies))
	mux.HandleFunc("/api/log-patterns", api.cors(api.handleLogPatterns))
	mux.HandleFunc("/api/correlations", api.cors(api.handleCorrelations))
	mux.HandleFunc("/api/reports", api.cors(api.handleReports))
	mux.HandleFunc("/api/reports/send", api.cors(api.handleSendReport))
	mux.HandleFunc("/api/stats", api.cors(api.handleStats))
	mux.HandleFunc("/api/benchmark", api.cors(api.handleBenchmark))
	mux.HandleFunc("/api/components/", api.cors(api.handleComponentAction))
	mux.HandleFunc("/api/correlations/compressed", api.cors(api.handleCompressedCorrelations))
	mux.HandleFunc("/api/scores", api.cors(api.handleScores))
	mux.HandleFunc("/api/scores/config", api.cors(api.handleScoresConfig))
	mux.HandleFunc("/api/scores/replay", api.cors(api.handleScoresReplay))

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	api.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := api.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			stdlog.Printf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the HTTP server.
func (api *BenchAPI) Stop() error {
	if api.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return api.server.Shutdown(ctx)
}

// cors wraps a handler with CORS headers.
func (api *BenchAPI) cors(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler(w, r)
	}
}

type parsedLogTagFilter struct {
	include map[string]map[string]struct{}
	exclude map[string]struct{}
}

type logsQuery struct {
	level       string
	kind        string
	startMs     int64
	endMs       int64
	limit       int
	offset      int
	tagFilter   parsedLogTagFilter
	patternHash string
}

func parseLogsQuery(query url.Values) logsQuery {
	result := logsQuery{
		level:       query.Get("level"),
		kind:        query.Get("kind"),
		limit:       1000,
		tagFilter:   parseLogTagFilter(query.Get("tags")),
		patternHash: query.Get("pattern"),
	}
	if result.kind == "" {
		result.kind = "all"
	}

	if s := query.Get("start"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			result.startMs = v
		}
	}
	if s := query.Get("end"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			result.endMs = v
		}
	}
	if s := query.Get("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			result.limit = v
		}
	}
	if result.limit > 5000 {
		result.limit = 5000
	}
	if s := query.Get("offset"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v >= 0 {
			result.offset = v
		}
	}

	return result
}

func parseLogTagFilter(input string) parsedLogTagFilter {
	filter := parsedLogTagFilter{
		include: make(map[string]map[string]struct{}),
		exclude: make(map[string]struct{}),
	}
	for _, token := range strings.Fields(strings.TrimSpace(input)) {
		if strings.HasPrefix(token, "-") && len(token) > 1 {
			filter.exclude[token[1:]] = struct{}{}
			continue
		}
		idx := strings.Index(token, ":")
		if idx <= 0 || idx == len(token)-1 {
			continue
		}
		key := token[:idx]
		if _, ok := filter.include[key]; !ok {
			filter.include[key] = make(map[string]struct{})
		}
		filter.include[key][token] = struct{}{}
	}
	return filter
}

func effectiveLogTags(logView observerdef.LogView) []string {
	tags := append([]string{}, logView.Tags()...)
	if tags == nil {
		tags = []string{}
	}

	statusTag := "status:" + strings.ToLower(logView.GetStatus())
	hasStatus := false
	for _, tag := range tags {
		if tag == statusTag {
			hasStatus = true
			break
		}
	}
	if !hasStatus {
		tags = append(tags, statusTag)
	}

	if host := logView.GetHostname(); host != "" {
		hostTag := "host:" + host
		hasHost := false
		for _, tag := range tags {
			if tag == hostTag {
				hasHost = true
				break
			}
		}
		if !hasHost {
			tags = append(tags, hostTag)
		}
	}

	return tags
}

func matchesLogTagFilter(tags []string, filter parsedLogTagFilter) bool {
	if len(filter.include) == 0 && len(filter.exclude) == 0 {
		return true
	}

	tagSet := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tagSet[tag] = struct{}{}
	}

	for excluded := range filter.exclude {
		if strings.Contains(excluded, ":") {
			if _, ok := tagSet[excluded]; ok {
				return false
			}
			continue
		}
		prefix := excluded + ":"
		for tag := range tagSet {
			if strings.HasPrefix(tag, prefix) {
				return false
			}
		}
	}

	for _, values := range filter.include {
		matched := false
		for value := range values {
			if _, ok := tagSet[value]; ok {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	return true
}

func matchesLogsQuery(logView observerdef.LogView, query logsQuery) bool {
	if query.level != "" && logView.GetStatus() != query.level {
		return false
	}
	isTelemetry := false
	for _, tag := range logView.Tags() {
		if tag == "telemetry:true" {
			isTelemetry = true
			break
		}
	}
	switch query.kind {
	case "raw":
		if isTelemetry {
			return false
		}
	case "telemetry":
		if !isTelemetry {
			return false
		}
	}
	ts := logView.GetTimestampUnixMilli()
	if query.startMs != 0 && ts < query.startMs {
		return false
	}
	if query.endMs != 0 && ts > query.endMs {
		return false
	}
	return matchesLogTagFilter(effectiveLogTags(logView), query.tagFilter)
}

func cloneCompressedGroups(groups []observerimpl.CompressedGroup) []observerimpl.CompressedGroup {
	cloned := make([]observerimpl.CompressedGroup, len(groups))
	for i, group := range groups {
		cloned[i] = group
		if group.CommonTags != nil {
			cloned[i].CommonTags = make(map[string]string, len(group.CommonTags))
			for key, value := range group.CommonTags {
				cloned[i].CommonTags[key] = value
			}
		}
		cloned[i].Patterns = append([]observerimpl.MetricPattern(nil), group.Patterns...)
		cloned[i].MemberSources = append([]string(nil), group.MemberSources...)
	}
	return cloned
}

// handleSSE serves a Server-Sent Events stream.
func (api *BenchAPI) handleSSE(w http.ResponseWriter, r *http.Request) {
	if api.tb.sseAccess == nil {
		http.Error(w, "SSE not available in headless mode", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	client, unsubscribe := api.tb.sseAccess.Subscribe()
	defer unsubscribe()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-client.StatusNotify:
			data := api.tb.sseAccess.LatestStatus()
			if data != nil {
				fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
				flusher.Flush()
			}
		case msg := <-client.Events:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, msg.Data)
			flusher.Flush()
		}
	}
}

// handleProgress returns replay progress.
func (api *BenchAPI) handleProgress(w http.ResponseWriter, _ *http.Request) {
	api.writeJSON(w, api.tb.debug.GetReplayProgress())
}

// handleStatus returns the current status.
func (api *BenchAPI) handleStatus(w http.ResponseWriter, _ *http.Request) {
	api.writeJSON(w, api.tb.GetStatus())
}

// handleScenarios lists available scenarios.
func (api *BenchAPI) handleScenarios(w http.ResponseWriter, _ *http.Request) {
	scenarios, err := api.tb.ListScenarios()
	if err != nil {
		api.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.writeJSON(w, scenarios)
}

// handleScenarioAction handles scenario-specific actions.
func (api *BenchAPI) handleScenarioAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/scenarios/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		api.writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	scenarioName := parts[0]
	action := parts[1]

	switch action {
	case "load":
		if r.Method != "POST" {
			api.writeError(w, http.StatusMethodNotAllowed, "use POST to load scenario")
			return
		}
		if err := api.tb.LoadScenario(scenarioName); err != nil {
			api.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.writeJSON(w, map[string]string{"status": "loaded", "scenario": scenarioName})
	default:
		api.writeError(w, http.StatusBadRequest, "unknown action: "+action)
	}
}

// handleComponents returns registered components.
func (api *BenchAPI) handleComponents(w http.ResponseWriter, _ *http.Request) {
	api.writeJSON(w, api.tb.GetComponents())
}

// handleSeriesList returns all available series.
func (api *BenchAPI) handleSeriesList(w http.ResponseWriter, _ *http.Request) {
	sv := api.tb.getStateView()
	if sv == nil {
		api.writeJSON(w, []interface{}{})
		return
	}

	storage := &stateViewStorage{sv: sv}

	type seriesInfo struct {
		ID         string   `json:"id"`
		Namespace  string   `json:"namespace"`
		Name       string   `json:"name"`
		Tags       []string `json:"tags"`
		PointCount int      `json:"pointCount"`
		Virtual    bool     `json:"virtual"`
		MetricKind string   `json:"metricKind,omitempty"`
	}

	var allSeries []seriesInfo
	extractorNs := api.tb.extractorNamespaces()

	for _, ns := range storage.listNamespaces() {
		metas := storage.listSeriesForNamespace(ns)
		for _, m := range metas {
			var aggs []observerdef.Aggregate
			if m.Namespace == "telemetry" {
				aggs = []observerdef.Aggregate{observerdef.AggregateSum}
			} else {
				aggs = []observerdef.Aggregate{observerdef.AggregateAverage, observerdef.AggregateCount}
			}
			var metricKind string
			if m.Namespace == "telemetry" {
				metricKind = "gauge"
			}
			for _, agg := range aggs {
				aggStr := aggSuffix(agg)
				nameWithAgg := m.Name + ":" + aggStr
				compactID := strconv.Itoa(int(m.Ref)) + ":" + aggStr
				_, virtual := extractorNs[m.Namespace]

				// Estimate point count from series data.
				s := sv.GetSeriesRange(m.Ref, 0, sv.MaxTimestamp(), agg)
				pointCount := 0
				if s != nil {
					pointCount = len(s.Points)
				}

				allSeries = append(allSeries, seriesInfo{
					ID:         compactID,
					Namespace:  m.Namespace,
					Name:       nameWithAgg,
					Tags:       m.Tags,
					PointCount: pointCount,
					Virtual:    virtual,
					MetricKind: metricKind,
				})
			}
		}
	}

	api.writeJSON(w, allSeries)
}

// handleSeriesDataByID returns data for a specific series by ID.
func (api *BenchAPI) handleSeriesDataByID(w http.ResponseWriter, r *http.Request) {
	encodedID := strings.TrimPrefix(r.URL.Path, "/api/series/id/")
	if encodedID == "" {
		api.writeError(w, http.StatusBadRequest, "path should be /api/series/id/{id}")
		return
	}
	seriesID, err := url.PathUnescape(encodedID)
	if err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid series id encoding")
		return
	}

	if colonIdx := strings.LastIndex(seriesID, ":"); colonIdx > 0 {
		prefix := seriesID[:colonIdx]
		if numericID, parseErr := strconv.Atoi(prefix); parseErr == nil {
			aggStr := seriesID[colonIdx+1:]
			api.handleNumericSeriesData(w, observerdef.SeriesRef(numericID), aggStr, seriesID)
			return
		}
	}

	namespace, nameWithAgg, tags, ok := parseSeriesKey(seriesID)
	if !ok {
		api.writeError(w, http.StatusBadRequest, "invalid series id")
		return
	}
	api.handleSeriesDataForSeries(w, namespace, nameWithAgg, tags, seriesID)
}

// handleNumericSeriesData resolves a compact numeric ID to series data.
func (api *BenchAPI) handleNumericSeriesData(w http.ResponseWriter, numericID observerdef.SeriesRef, aggStr string, originalID string) {
	var agg observerdef.Aggregate
	switch aggStr {
	case "avg":
		agg = observerdef.AggregateAverage
	case "count":
		agg = observerdef.AggregateCount
	case "sum":
		agg = observerdef.AggregateSum
	case "min":
		agg = observerdef.AggregateMin
	case "max":
		agg = observerdef.AggregateMax
	default:
		api.writeError(w, http.StatusBadRequest, "invalid aggregation suffix")
		return
	}

	sv := api.tb.getStateView()
	if sv == nil {
		api.writeError(w, http.StatusServiceUnavailable, "no data loaded")
		return
	}
	storage := &stateViewStorage{sv: sv}

	series := storage.getSeriesByNumericID(numericID, agg)
	if series == nil {
		api.writeError(w, http.StatusNotFound, "series not found")
		return
	}

	meta := storage.getSeriesMeta(numericID)
	if meta == nil {
		api.writeError(w, http.StatusNotFound, "series metadata not found")
		return
	}

	nameWithAgg := series.Name + ":" + aggStr

	sd := observerdef.SeriesDescriptor{
		Namespace: meta.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Aggregate: agg,
	}
	anomalies := api.tb.GetMetricsAnomaliesForSource(sd)

	type anomalyMarker struct {
		Timestamp         int64  `json:"timestamp"`
		DetectorName      string `json:"detectorName"`
		DetectorComponent string `json:"detectorComponent"`
		SourceSeriesID    string `json:"sourceSeriesId"`
		Title             string `json:"title"`
	}

	var markers []anomalyMarker
	detectorComponentMap := api.tb.GetDetectorComponentMap()
	for _, a := range anomalies {
		if a.DetectorName == "" || a.Timestamp == 0 {
			continue
		}
		markers = append(markers, anomalyMarker{
			Timestamp:         a.Timestamp,
			DetectorName:      a.DetectorName,
			DetectorComponent: detectorComponentMap[a.DetectorName],
			SourceSeriesID:    originalID,
			Title:             a.Title,
		})
	}

	type pointOutput struct {
		Timestamp int64   `json:"timestamp"`
		Value     float64 `json:"value"`
	}

	type seriesResponse struct {
		ID        string          `json:"id"`
		Namespace string          `json:"namespace"`
		Name      string          `json:"name"`
		Tags      []string        `json:"tags"`
		Points    []pointOutput   `json:"points"`
		Anomalies []anomalyMarker `json:"anomalies"`
	}

	resp := seriesResponse{
		ID:        originalID,
		Namespace: meta.Namespace,
		Name:      nameWithAgg,
		Tags:      series.Tags,
		Points:    make([]pointOutput, len(series.Points)),
		Anomalies: markers,
	}

	for i, p := range series.Points {
		value := p.Value
		if math.IsInf(value, 0) || math.IsNaN(value) {
			value = 0
		}
		resp.Points[i] = pointOutput{Timestamp: p.Timestamp, Value: value}
	}

	api.writeJSON(w, resp)
}

// handleSeriesData returns data for a specific series by namespace/name path.
func (api *BenchAPI) handleSeriesData(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/series/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) < 2 {
		api.writeError(w, http.StatusBadRequest, "path should be /api/series/{namespace}/{name}")
		return
	}

	namespace := parts[0]
	nameWithAgg := parts[1]
	api.handleSeriesDataForSeries(w, namespace, nameWithAgg, nil, "")
}

func (api *BenchAPI) handleSeriesDataForSeries(w http.ResponseWriter, namespace, nameWithAgg string, tags []string, requestedID string) {
	seriesID := requestedID

	name := nameWithAgg
	agg := observerdef.AggregateAverage
	if idx := strings.LastIndex(nameWithAgg, ":"); idx != -1 {
		suffix := nameWithAgg[idx+1:]
		name = nameWithAgg[:idx]
		switch suffix {
		case "avg":
			agg = observerdef.AggregateAverage
		case "count":
			agg = observerdef.AggregateCount
		case "sum":
			agg = observerdef.AggregateSum
		case "min":
			agg = observerdef.AggregateMin
		case "max":
			agg = observerdef.AggregateMax
		}
	}

	sv := api.tb.getStateView()
	if sv == nil {
		api.writeError(w, http.StatusServiceUnavailable, "no data loaded")
		return
	}
	storage := &stateViewStorage{sv: sv}

	// Find the series by namespace+name+tags.
	metas := storage.listSeriesForNamespace(namespace)
	var foundMeta *observerdef.SeriesMeta
	for i := range metas {
		m := &metas[i]
		if m.Name != name {
			continue
		}
		if tags == nil || tagsMatch(m.Tags, tags) {
			foundMeta = m
			break
		}
	}

	if foundMeta == nil {
		api.writeError(w, http.StatusNotFound, "series not found")
		return
	}

	series := storage.getSeriesByNumericID(foundMeta.Ref, agg)
	if series == nil {
		api.writeError(w, http.StatusNotFound, "series not found")
		return
	}

	if seriesID == "" {
		seriesID = strconv.Itoa(int(foundMeta.Ref)) + ":" + aggSuffix(agg)
	}

	type anomalyMarker struct {
		Timestamp         int64  `json:"timestamp"`
		DetectorName      string `json:"detectorName"`
		DetectorComponent string `json:"detectorComponent"`
		SourceSeriesID    string `json:"sourceSeriesId"`
		Title             string `json:"title"`
	}

	var markers []anomalyMarker
	sd := observerdef.SeriesDescriptor{
		Namespace: namespace,
		Name:      name,
		Tags:      foundMeta.Tags,
		Aggregate: agg,
	}
	anomalies := api.tb.GetMetricsAnomaliesForSource(sd)
	detectorComponentMap := api.tb.GetDetectorComponentMap()
	for _, a := range anomalies {
		if a.DetectorName == "" || a.Timestamp == 0 {
			stdlog.Printf("skipping malformed anomaly marker for series %q: detector=%q ts=%d",
				seriesID, a.DetectorName, a.Timestamp)
			continue
		}
		markers = append(markers, anomalyMarker{
			Timestamp:         a.Timestamp,
			DetectorName:      a.DetectorName,
			DetectorComponent: detectorComponentMap[a.DetectorName],
			SourceSeriesID:    seriesID,
			Title:             a.Title,
		})
	}

	type pointOutput struct {
		Timestamp int64   `json:"timestamp"`
		Value     float64 `json:"value"`
	}

	type seriesResponse struct {
		ID        string          `json:"id"`
		Namespace string          `json:"namespace"`
		Name      string          `json:"name"`
		Tags      []string        `json:"tags"`
		Points    []pointOutput   `json:"points"`
		Anomalies []anomalyMarker `json:"anomalies"`
	}

	resp := seriesResponse{
		ID:        seriesID,
		Namespace: namespace,
		Name:      nameWithAgg,
		Tags:      foundMeta.Tags,
		Points:    make([]pointOutput, len(series.Points)),
		Anomalies: markers,
	}

	for i, p := range series.Points {
		value := p.Value
		if math.IsInf(value, 0) || math.IsNaN(value) {
			value = 0
		}
		resp.Points[i] = pointOutput{Timestamp: p.Timestamp, Value: value}
	}

	api.writeJSON(w, resp)
}

// handleAnomalies returns all detected anomalies.
func (api *BenchAPI) handleAnomalies(w http.ResponseWriter, r *http.Request) {
	detectorFilter := r.URL.Query().Get("detector")

	type debugInfoResponse struct {
		BaselineStart  int64     `json:"baselineStart"`
		BaselineEnd    int64     `json:"baselineEnd"`
		BaselineMean   float64   `json:"baselineMean,omitempty"`
		BaselineMedian float64   `json:"baselineMedian,omitempty"`
		BaselineStddev float64   `json:"baselineStddev,omitempty"`
		BaselineMAD    float64   `json:"baselineMAD,omitempty"`
		Threshold      float64   `json:"threshold"`
		SlackParam     float64   `json:"slackParam,omitempty"`
		CurrentValue   float64   `json:"currentValue"`
		DeviationSigma float64   `json:"deviationSigma"`
		CUSUMValues    []float64 `json:"cusumValues,omitempty"`
	}

	type anomalyResponse struct {
		Source            string             `json:"source"`
		SourceSeriesID    string             `json:"sourceSeriesId"`
		DetectorName      string             `json:"detectorName"`
		DetectorComponent string             `json:"detectorComponent"`
		Title             string             `json:"title"`
		Description       string             `json:"description"`
		Tags              []string           `json:"tags"`
		Timestamp         int64              `json:"timestamp"`
		DebugInfo         *debugInfoResponse `json:"debugInfo,omitempty"`
	}

	detectorComponentMap := api.tb.GetDetectorComponentMap()
	sv := api.tb.getStateView()

	resolveCompactID := func(a observerdef.Anomaly) string {
		if a.SourceRef != nil {
			return a.SourceRef.CompactID()
		}
		if sv != nil && a.DetectorName != "" && a.Source.Name != "" {
			storage := &stateViewStorage{sv: sv}
			telemetryName := "telemetry." + a.DetectorName + "." + a.Source.String()
			key := seriesKey("telemetry", telemetryName+":avg", nil)
			if compactID := storage.compactSeriesID(key); compactID != key {
				return compactID
			}
		}
		return a.Source.Key()
	}

	toResponse := func(a observerdef.Anomaly) anomalyResponse {
		resp := anomalyResponse{
			Source:            a.Source.String(),
			SourceSeriesID:    resolveCompactID(a),
			DetectorName:      a.DetectorName,
			DetectorComponent: detectorComponentMap[a.DetectorName],
			Title:             a.Title,
			Description:       a.Description,
			Tags:              a.Source.Tags,
			Timestamp:         a.Timestamp,
		}
		if a.DebugInfo != nil {
			resp.DebugInfo = &debugInfoResponse{
				BaselineStart:  a.DebugInfo.BaselineStart,
				BaselineEnd:    a.DebugInfo.BaselineEnd,
				BaselineMean:   a.DebugInfo.BaselineMean,
				BaselineMedian: a.DebugInfo.BaselineMedian,
				BaselineStddev: a.DebugInfo.BaselineStddev,
				BaselineMAD:    a.DebugInfo.BaselineMAD,
				Threshold:      a.DebugInfo.Threshold,
				SlackParam:     a.DebugInfo.SlackParam,
				CurrentValue:   a.DebugInfo.CurrentValue,
				DeviationSigma: a.DebugInfo.DeviationSigma,
				CUSUMValues:    a.DebugInfo.CUSUMValues,
			}
		}
		return resp
	}

	var response []anomalyResponse

	if detectorFilter != "" {
		byDetector := api.tb.GetMetricsAnomaliesByDetector()
		if anomalies, ok := byDetector[detectorFilter]; ok {
			for _, a := range anomalies {
				if a.DetectorName == "" || a.Timestamp == 0 {
					continue
				}
				response = append(response, toResponse(a))
			}
		}
	} else {
		anomalies := api.tb.GetMetricsAnomalies()
		for _, a := range anomalies {
			if a.DetectorName == "" || a.Timestamp == 0 {
				continue
			}
			response = append(response, toResponse(a))
		}
	}

	api.writeJSON(w, response)
}

// handleLogAnomalies returns anomalies from log detectors.
func (api *BenchAPI) handleLogAnomalies(w http.ResponseWriter, r *http.Request) {
	detectorFilter := r.URL.Query().Get("detector")

	type logAnomalyResponse struct {
		Source       string   `json:"source"`
		DetectorName string   `json:"detectorName"`
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		Tags         []string `json:"tags"`
		Timestamp    int64    `json:"timestamp"`
		Score        *float64 `json:"score,omitempty"`
	}

	var anomalies []observerdef.Anomaly
	if detectorFilter != "" {
		byDetector := api.tb.GetLogAnomaliesByDetector()
		anomalies = byDetector[detectorFilter]
	} else {
		anomalies = api.tb.GetLogAnomalies()
	}

	response := make([]logAnomalyResponse, 0, len(anomalies))
	for _, a := range anomalies {
		response = append(response, logAnomalyResponse{
			Source:       a.Source.String(),
			DetectorName: a.DetectorName,
			Title:        a.Title,
			Description:  a.Description,
			Tags:         a.Source.Tags,
			Timestamp:    a.Timestamp,
			Score:        a.Score,
		})
	}

	api.writeJSON(w, response)
}

// handleLogs returns raw log entries with filtering and pagination.
func (api *BenchAPI) handleLogs(w http.ResponseWriter, r *http.Request) {
	query := parseLogsQuery(r.URL.Query())

	type logEntryResponse struct {
		TimestampMs int64    `json:"timestampMs"`
		Status      string   `json:"status"`
		Content     string   `json:"content"`
		Hostname    string   `json:"hostname"`
		Tags        []string `json:"tags"`
	}

	logs := api.tb.GetRawLogs()

	total := 0
	result := make([]logEntryResponse, 0, query.limit)
	for _, l := range logs {
		if !matchesLogsQuery(l, query) {
			continue
		}
		if total >= query.offset && len(result) < query.limit {
			tags := effectiveLogTags(l)
			ts := l.GetTimestampUnixMilli()
			result = append(result, logEntryResponse{
				TimestampMs: ts,
				Status:      l.GetStatus(),
				Content:     string(l.GetContent()),
				Hostname:    l.GetHostname(),
				Tags:        tags,
			})
		}
		total++
	}

	api.writeJSON(w, map[string]interface{}{
		"logs":   result,
		"total":  total,
		"limit":  query.limit,
		"offset": query.offset,
	})
}

// handleLogPatterns returns the list of log patterns.
func (api *BenchAPI) handleLogPatterns(w http.ResponseWriter, _ *http.Request) {
	api.writeJSON(w, api.tb.GetLogPatterns())
}

// handleLogsSummary returns lightweight summary data about logs.
func (api *BenchAPI) handleLogsSummary(w http.ResponseWriter, r *http.Request) {
	query := parseLogsQuery(r.URL.Query())
	logs := api.tb.GetRawLogs()

	totalCount := 0
	countByLevel := make(map[string]int)
	tagGroups := make(map[string]map[string]struct{})
	var minTs, maxTs int64

	for _, l := range logs {
		if !matchesLogsQuery(l, query) {
			continue
		}
		totalCount++
		countByLevel[l.GetStatus()]++
		ts := l.GetTimestampUnixMilli()
		if minTs == 0 || ts < minTs {
			minTs = ts
		}
		if ts > maxTs {
			maxTs = ts
		}
		for _, tag := range effectiveLogTags(l) {
			if idx := strings.Index(tag, ":"); idx > 0 {
				key := tag[:idx]
				if _, ok := tagGroups[key]; !ok {
					tagGroups[key] = make(map[string]struct{})
				}
				tagGroups[key][tag] = struct{}{}
			}
		}
	}

	const numBuckets = 100
	type histBucket struct {
		TimestampMs int64 `json:"timestampMs"`
		Count       int   `json:"count"`
	}

	histogram := make([]histBucket, 0)
	if totalCount > 0 && maxTs > minTs {
		bucketWidth := (maxTs - minTs + numBuckets) / numBuckets
		histogram = make([]histBucket, numBuckets)
		for i := range histogram {
			histogram[i].TimestampMs = minTs + int64(i)*bucketWidth
		}
		for _, l := range logs {
			if !matchesLogsQuery(l, query) {
				continue
			}
			ts := l.GetTimestampUnixMilli()
			idx := int((ts - minTs) / bucketWidth)
			if idx >= numBuckets {
				idx = numBuckets - 1
			}
			histogram[idx].Count++
		}
	} else if totalCount > 0 {
		histogram = []histBucket{{TimestampMs: minTs, Count: totalCount}}
	}

	sortedTagGroups := make(map[string][]string, len(tagGroups))
	for key, values := range tagGroups {
		group := make([]string, 0, len(values))
		for value := range values {
			group = append(group, value)
		}
		sort.Strings(group)
		sortedTagGroups[key] = group
	}

	api.writeJSON(w, map[string]interface{}{
		"totalCount":   totalCount,
		"countByLevel": countByLevel,
		"timeRange": map[string]int64{
			"start": minTs,
			"end":   maxTs,
		},
		"histogram": histogram,
		"tagGroups": sortedTagGroups,
	})
}

// handleCorrelations returns detected correlations.
func (api *BenchAPI) handleCorrelations(w http.ResponseWriter, _ *http.Request) {
	correlations := api.tb.GetCorrelations()

	type anomalyOutput struct {
		Source      string   `json:"source"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Timestamp   int64    `json:"timestamp"`
		Score       *float64 `json:"score,omitempty"`
		Tags        []string `json:"tags"`
	}

	type correlationResponse struct {
		Pattern         string          `json:"pattern"`
		Title           string          `json:"title"`
		MemberSeriesIDs []string        `json:"memberSeriesIds"`
		MetricNames     []string        `json:"metricNames"`
		Anomalies       []anomalyOutput `json:"anomalies"`
		FirstSeen       int64           `json:"firstSeen"`
		LastUpdated     int64           `json:"lastUpdated"`
	}

	response := make([]correlationResponse, len(correlations))
	for i, c := range correlations {
		anomalies := make([]anomalyOutput, len(c.Anomalies))
		for j, a := range c.Anomalies {
			tgs := a.Source.Tags
			if tgs == nil {
				tgs = []string{}
			}
			anomalies[j] = anomalyOutput{
				Source:      a.Source.String(),
				Title:       a.Title,
				Description: a.Description,
				Timestamp:   a.Timestamp,
				Score:       a.Score,
				Tags:        tgs,
			}
		}

		memberIDs := make([]string, len(c.Members))
		for k, m := range c.Members {
			// Find SourceRef for this member.
			for _, a := range c.Anomalies {
				if a.Source.Key() == m.Key() && a.SourceRef != nil {
					memberIDs[k] = a.SourceRef.CompactID()
					break
				}
			}
			if memberIDs[k] == "" {
				memberIDs[k] = m.Key()
			}
		}

		metricNames := make([]string, len(c.Members))
		for k, m := range c.Members {
			metricNames[k] = m.String()
		}

		response[i] = correlationResponse{
			Pattern:         c.Pattern,
			Title:           c.Title,
			MemberSeriesIDs: memberIDs,
			MetricNames:     metricNames,
			Anomalies:       anomalies,
			FirstSeen:       c.FirstSeen,
			LastUpdated:     c.LastUpdated,
		}
	}

	api.writeJSON(w, response)
}

// handleStats returns correlator statistics.
func (api *BenchAPI) handleStats(w http.ResponseWriter, _ *http.Request) {
	api.writeJSON(w, api.tb.GetCorrelatorStats())
}

// handleReports returns reported events.
func (api *BenchAPI) handleReports(w http.ResponseWriter, _ *http.Request) {
	events := api.tb.GetReportedEvents()
	if events == nil {
		events = []ReportedEvent{}
	}
	api.writeJSON(w, events)
}

// handleSendReport posts a specific ReportedEvent.
func (api *BenchAPI) handleSendReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		api.writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var req struct {
		Pattern   string `json:"pattern"`
		FirstSeen int64  `json:"firstSeen"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Pattern == "" {
		api.writeError(w, http.StatusBadRequest, "pattern is required")
		return
	}
	if err := api.tb.SendReportedEvent(req.Pattern, req.FirstSeen); err != nil {
		api.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.writeJSON(w, map[string]string{"status": "sent"})
}

// handleComponentAction handles /api/components/{name}/{action}.
func (api *BenchAPI) handleComponentAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/components/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		api.writeError(w, http.StatusBadRequest, "expected /api/components/{name}/{action}")
		return
	}

	name := parts[0]
	action := parts[1]

	switch action {
	case "toggle":
		if r.Method != "POST" {
			api.writeError(w, http.StatusMethodNotAllowed, "use POST to toggle")
			return
		}
		if err := api.tb.ToggleComponent(name); err != nil {
			api.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		api.writeJSON(w, api.tb.GetStatus())
	case "data":
		if r.Method != "GET" {
			api.writeError(w, http.StatusMethodNotAllowed, "use GET for component data")
			return
		}
		data, enabled := api.tb.GetComponentData(name)
		api.writeJSON(w, map[string]interface{}{
			"enabled": enabled,
			"data":    data,
		})
	default:
		api.writeError(w, http.StatusBadRequest, "unknown action: "+action)
	}
}

// handleCompressedCorrelations returns compressed group descriptions.
func (api *BenchAPI) handleCompressedCorrelations(w http.ResponseWriter, r *http.Request) {
	threshold := 0.75
	if t := r.URL.Query().Get("threshold"); t != "" {
		if parsed, err := strconv.ParseFloat(t, 64); err == nil && parsed > 0 && parsed <= 1 {
			threshold = parsed
		}
	}
	groups := cloneCompressedGroups(api.tb.GetCompressedCorrelations(threshold))

	// Translate MemberSources from full keys to compact numeric IDs.
	sv := api.tb.getStateView()
	if sv != nil {
		storage := &stateViewStorage{sv: sv}
		for i := range groups {
			for j, src := range groups[i].MemberSources {
				groups[i].MemberSources[j] = storage.compactSeriesID(src)
			}
		}
	}

	api.writeJSON(w, groups)
}

// handleBenchmark returns replay statistics.
func (api *BenchAPI) handleBenchmark(w http.ResponseWriter, _ *http.Request) {
	stats := api.tb.GetReplayStats()
	if stats == nil {
		api.writeJSON(w, &ReplayStats{DetectorStats: map[string]DetectorProcessingStats{}})
		return
	}
	api.writeJSON(w, stats)
}

// handleScores returns the current AnomalyScoreState from the live scorer.
// GET /api/scores
func (api *BenchAPI) handleScores(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	sv := api.tb.getStateView()
	if sv == nil {
		api.writeJSON(w, observerdef.AnomalyScoreState{})
		return
	}
	api.writeJSON(w, sv.ScoreState())
}

// handleScoresConfig returns the server-side default AnomalyScorerConfig so the UI
// never needs to hardcode threshold values. The response also includes
// cooldown so the Scorer tab can initialise its replay form correctly.
// GET /api/scores/config
func (api *BenchAPI) handleScoresConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeError(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	api.writeJSON(w, observerimpl.DefaultAnomalyScorerConfig())
}

// handleScoresReplay re-runs the scorer over the full retained raw-anomaly set
// using a config provided in the POST body. This lets the UI inspect scorer
// output without re-running detectors.
//
// The testbench simulates the live agent's 1-second timer: anomalies are sorted
// by timestamp and fed second-by-second, with Advance called once per unique
// second (and for any empty seconds in between).
//
// POST /api/scores/replay   body: { AnomalyScorerConfig fields... , "cooldown": N }
func (api *BenchAPI) handleScoresReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.writeError(w, http.StatusMethodNotAllowed, "use POST")
		return
	}

	// replayRequest embeds the EWMA config fields at the top level and adds
	// CooldownSecs for the subscription, preserving the flat JSON wire format
	// the UI sends.
	type replayRequest struct {
		observerdef.AnomalyScorerConfig
		CooldownSecs int64 `json:"cooldown_secs"`
	}
	defaults := observerimpl.DefaultAnomalyScorerConfig()
	req := replayRequest{AnomalyScorerConfig: defaults.AnomalyScorerConfig, CooldownSecs: defaults.CooldownSecs}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid config: "+err.Error())
		return
	}
	// Replay always keeps all buckets so the UI can render the full time range.
	// math.MaxInt64 signals "unlimited" to the trim logic (which defaults to WindowSecs when 0).
	req.AnomalyScorerConfig.MaxBuckets = math.MaxInt64

	sv := api.tb.getStateView()
	if sv == nil {
		api.writeJSON(w, observerdef.AnomalyScoreState{})
		return
	}

	// Include all anomalies with a valid timestamp, matching live-agent behaviour.
	// Log anomalies have no entry in DetectorThresholds and fall through to the
	// default level (Medium) — the same as any metric detector without explicit
	// thresholds. Filtering them out would diverge from production.
	raw := sv.Anomalies()
	anomalies := make([]observerdef.Anomaly, 0, len(raw))
	for _, a := range raw {
		if a.Timestamp == 0 {
			continue
		}
		anomalies = append(anomalies, a)
	}
	if len(anomalies) == 0 {
		api.writeJSON(w, observerdef.AnomalyScoreState{Config: req.AnomalyScorerConfig})
		return
	}

	// Sort anomalies by timestamp so the scorer processes time monotonically.
	//
	// Known limitation: scan detectors (scanmw, scanwelch) emit changepoint timestamps
	// that are historically earlier than the engine tick at which the anomaly was
	// produced. In the live agent, ProcessAnomaly clamps such anomalies to
	// lastAdvancedSec+1 via the engine detection tick. Faithfully replicating that
	// requires storing the engine arrival time alongside the anomaly timestamp, which
	// Anomaly.Timestamp does not currently encode. Sorting by Timestamp places scan
	// anomalies at their changepoint second rather than their detection second; this
	// is a known approximation in the Scorer tab for scan-detector scenarios.
	sorted := make([]observerdef.Anomaly, len(anomalies))
	copy(sorted, anomalies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp < sorted[j].Timestamp
	})

	scorerCfg := observerimpl.AnomalyScorerConfig{AnomalyScorerConfig: req.AnomalyScorerConfig}
	scorer := observerimpl.NewAnomalyScorer(scorerCfg)

	first := sorted[0].Timestamp
	last := sorted[len(sorted)-1].Timestamp

	collector := &scorerEventCollector{}
	subscription, err := scorer.SubscribeSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: req.CooldownSecs,
	}, collector)
	if err != nil {
		api.writeError(w, http.StatusInternalServerError, "subscribe severity events: "+err.Error())
		return
	}
	defer subscription.Unsubscribe()

	ai := 0
	for sec := first; sec <= last; sec++ {
		for ai < len(sorted) && sorted[ai].Timestamp == sec {
			scorer.ProcessAnomaly(sorted[ai])
			ai++
		}
		scorer.Advance(sec)
	}

	// Return a wrapper that adds the collected events alongside the state snapshot.
	state := scorer.ScoreState()
	api.writeJSON(w, struct {
		observerdef.AnomalyScoreState
		Events []severityeventsdef.SeverityEvent `json:"events"`
	}{AnomalyScoreState: state, Events: collector.events})
}

// scorerEventCollector implements severityeventsdef.SeverityEventListener, accumulating
// every severity transition fired by the scorer's per-subscription state machine.
type scorerEventCollector struct {
	events []severityeventsdef.SeverityEvent
}

func (c *scorerEventCollector) OnSeverityTransition(evt severityeventsdef.SeverityEvent) {
	c.events = append(c.events, evt)
}

// writeJSON writes a JSON response.
func (api *BenchAPI) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		stdlog.Printf("Failed to encode JSON: %v", err)
		http.Error(w, `{"error":"encoding error"}`, http.StatusInternalServerError)
	}
}

// writeError writes an error response.
func (api *BenchAPI) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// handleSSE needs the SSEClient struct from testbenchimpl - ensure it uses correct fields.
var _ = (*testbenchimpl.SSEClient)(nil)
