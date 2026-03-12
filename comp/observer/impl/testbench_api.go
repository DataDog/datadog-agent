// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TestBenchAPI handles HTTP API requests for the test bench.
type TestBenchAPI struct {
	tb     *TestBench
	server *http.Server
}

// NewTestBenchAPI creates a new API handler.
func NewTestBenchAPI(tb *TestBench) *TestBenchAPI {
	return &TestBenchAPI{tb: tb}
}

// Start starts the HTTP server.
func (api *TestBenchAPI) Start(addr string) error {
	mux := http.NewServeMux()

	// Wrap all handlers with CORS middleware
	mux.HandleFunc("/api/events", api.handleSSE) // SSE — no CORS wrapper (needs http.Flusher)
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
	mux.HandleFunc("/api/correlations", api.cors(api.handleCorrelations))
	mux.HandleFunc("/api/leadlag", api.cors(api.handleLeadLag))
	mux.HandleFunc("/api/surprise", api.cors(api.handleSurprise))
	mux.HandleFunc("/api/stats", api.cors(api.handleStats))
	mux.HandleFunc("/api/components/", api.cors(api.handleComponentAction))
	mux.HandleFunc("/api/correlations/compressed", api.cors(api.handleCompressedCorrelations))
	mux.HandleFunc("/api/score", api.cors(api.handleScore))

	api.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := api.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the HTTP server.
func (api *TestBenchAPI) Stop() error {
	if api.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return api.server.Shutdown(ctx)
}

// cors wraps a handler with CORS headers and request timing.
func (api *TestBenchAPI) cors(handler http.HandlerFunc) http.HandlerFunc {
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
	level     string
	kind      string
	startMs   int64
	endMs     int64
	limit     int
	offset    int
	tagFilter parsedLogTagFilter
}

func parseLogsQuery(query url.Values) logsQuery {
	result := logsQuery{
		level:     query.Get("level"),
		kind:      query.Get("kind"),
		limit:     1000,
		tagFilter: parseLogTagFilter(query.Get("tags")),
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
	tags := append([]string{}, logView.GetTags()...)
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
	for _, tag := range logView.GetTags() {
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

func cloneCompressedGroups(groups []CompressedGroup) []CompressedGroup {
	cloned := make([]CompressedGroup, len(groups))
	for i, group := range groups {
		cloned[i] = group
		if group.CommonTags != nil {
			cloned[i].CommonTags = make(map[string]string, len(group.CommonTags))
			for key, value := range group.CommonTags {
				cloned[i].CommonTags[key] = value
			}
		}
		cloned[i].Patterns = append([]MetricPattern(nil), group.Patterns...)
		cloned[i].MemberSources = append([]string(nil), group.MemberSources...)
	}
	return cloned
}

// handleSSE serves a Server-Sent Events stream for real-time updates.
func (api *TestBenchAPI) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe — if a status exists, the client's statusNotify is pre-signaled.
	client, unsubscribe := api.tb.sse.subscribe()
	defer unsubscribe()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-client.statusNotify:
			data := api.tb.sse.latestStatusData()
			if data != nil {
				fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
				flusher.Flush()
			}
		case msg := <-client.events:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, msg.Data)
			flusher.Flush()
		}
	}
}

// handleProgress returns replay progress counters (lock-free, safe to call during load).
func (api *TestBenchAPI) handleProgress(w http.ResponseWriter, _ *http.Request) {
	api.writeJSON(w, api.tb.engine.GetReplayProgress())
}

// handleStatus returns the current status.
func (api *TestBenchAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := api.tb.GetStatus()
	api.writeJSON(w, status)
}

// handleScenarios lists available scenarios.
func (api *TestBenchAPI) handleScenarios(w http.ResponseWriter, r *http.Request) {
	scenarios, err := api.tb.ListScenarios()
	if err != nil {
		api.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.writeJSON(w, scenarios)
}

// handleScenarioAction handles scenario-specific actions (load).
func (api *TestBenchAPI) handleScenarioAction(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/scenarios/{name}/load
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
func (api *TestBenchAPI) handleComponents(w http.ResponseWriter, r *http.Request) {
	components := api.tb.GetComponents()
	api.writeJSON(w, components)
}

// handleSeriesList returns all available series.
func (api *TestBenchAPI) handleSeriesList(w http.ResponseWriter, r *http.Request) {
	storage := api.tb.GetStorage()
	if storage == nil {
		api.writeJSON(w, []interface{}{})
		return
	}

	type seriesInfo struct {
		ID         string   `json:"id"`
		Namespace  string   `json:"namespace"`
		Name       string   `json:"name"`
		Tags       []string `json:"tags"`
		PointCount int      `json:"pointCount"`
	}

	var allSeries []seriesInfo

	// Get series metadata from all namespaces — no point data materialized.
	// Use compact numeric IDs: "{numericID}:{aggSuffix}" (e.g. "42:avg").
	for _, ns := range storage.Namespaces() {
		metas := storage.ListSeriesMetadata(ns)
		for _, m := range metas {
			for _, agg := range []Aggregate{AggregateAverage, AggregateCount} {
				nameWithAgg := m.Name + ":" + aggSuffix(agg)
				compactID := strconv.Itoa(m.ID) + ":" + aggSuffix(agg)
				allSeries = append(allSeries, seriesInfo{
					ID:         compactID,
					Namespace:  m.Namespace,
					Name:       nameWithAgg,
					Tags:       m.Tags,
					PointCount: m.PointCount,
				})
			}
		}
	}

	api.writeJSON(w, allSeries)
}

// handleSeriesDataByID returns data for a specific series by canonical id.
// Supports both compact numeric IDs ("42:avg") and legacy full key IDs.
func (api *TestBenchAPI) handleSeriesDataByID(w http.ResponseWriter, r *http.Request) {
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

	// Try compact numeric ID format: "{numericID}:{aggSuffix}" (e.g. "42:avg")
	if colonIdx := strings.LastIndex(seriesID, ":"); colonIdx > 0 {
		prefix := seriesID[:colonIdx]
		if numericID, parseErr := strconv.Atoi(prefix); parseErr == nil {
			aggStr := seriesID[colonIdx+1:]
			api.handleNumericSeriesData(w, numericID, aggStr, seriesID)
			return
		}
	}

	// Fall back to legacy full key format: "namespace|name:agg|tags"
	namespace, nameWithAgg, tags, ok := parseSeriesKey(seriesID)
	if !ok {
		api.writeError(w, http.StatusBadRequest, "invalid series id")
		return
	}
	api.handleSeriesDataForSeries(w, namespace, nameWithAgg, tags, observerdef.SeriesID(seriesID))
}

// handleNumericSeriesData resolves a compact numeric ID to series data.
func (api *TestBenchAPI) handleNumericSeriesData(w http.ResponseWriter, numericID int, aggStr string, originalID string) {
	var agg Aggregate
	switch aggStr {
	case "avg":
		agg = AggregateAverage
	case "count":
		agg = AggregateCount
	case "sum":
		agg = AggregateSum
	case "min":
		agg = AggregateMin
	case "max":
		agg = AggregateMax
	default:
		api.writeError(w, http.StatusBadRequest, "invalid aggregation suffix")
		return
	}

	storage := api.tb.GetStorage()
	if storage == nil {
		api.writeError(w, http.StatusServiceUnavailable, "no data loaded")
		return
	}

	series := storage.GetSeriesByNumericID(numericID, agg)
	if series == nil {
		api.writeError(w, http.StatusNotFound, "series not found")
		return
	}

	nameWithAgg := series.Name + ":" + aggStr
	seriesID := observerdef.SeriesID(originalID)

	// Look up anomalies using the full key format that detectors produce
	// (e.g. "parquet|metric:avg|tags"), not the compact numeric ID.
	fullKey, _ := storage.FullKeyForNumericID(numericID)
	var fullKeyWithAgg string
	if ns, name, tags, ok := parseSeriesKey(fullKey); ok {
		fullKeyWithAgg = seriesKey(ns, name+":"+aggStr, tags)
	}
	anomalyLookupID := seriesID
	if fullKeyWithAgg != "" {
		anomalyLookupID = observerdef.SeriesID(fullKeyWithAgg)
	}
	anomalies := api.tb.GetMetricsAnomaliesForSeries(anomalyLookupID)

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
			SourceSeriesID:    string(seriesID),
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
		ID:        string(seriesID),
		Namespace: series.Namespace,
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
		resp.Points[i] = pointOutput{
			Timestamp: p.Timestamp,
			Value:     value,
		}
	}

	api.writeJSON(w, resp)
}

// handleSeriesData returns data for a specific series.
func (api *TestBenchAPI) handleSeriesData(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/series/{namespace}/{name}
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

func (api *TestBenchAPI) handleSeriesDataForSeries(w http.ResponseWriter, namespace, nameWithAgg string, tags []string, requestedID observerdef.SeriesID) {
	seriesID := requestedID

	// Parse aggregation suffix (e.g., "metric:avg" or "metric:count")
	name := nameWithAgg
	agg := AggregateAverage
	if idx := strings.LastIndex(nameWithAgg, ":"); idx != -1 {
		suffix := nameWithAgg[idx+1:]
		name = nameWithAgg[:idx]
		switch suffix {
		case "avg":
			agg = AggregateAverage
		case "count":
			agg = AggregateCount
		case "sum":
			agg = AggregateSum
		case "min":
			agg = AggregateMin
		case "max":
			agg = AggregateMax
		}
	}

	storage := api.tb.GetStorage()
	if storage == nil {
		api.writeError(w, http.StatusServiceUnavailable, "no data loaded")
		return
	}

	series := storage.GetSeries(namespace, name, tags, agg)
	if series == nil {
		api.writeError(w, http.StatusNotFound, "series not found")
		return
	}
	if seriesID == "" {
		seriesID = observerdef.SeriesID(seriesKey(series.Namespace, nameWithAgg, series.Tags))
	}

	// Get anomalies for this series to include in response
	anomalies := api.tb.GetMetricsAnomaliesForSeries(seriesID)

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
			log.Printf("skipping malformed anomaly marker for series %q: detector=%q ts=%d",
				string(seriesID), a.DetectorName, a.Timestamp)
			continue
		}
		markers = append(markers, anomalyMarker{
			Timestamp:         a.Timestamp,
			DetectorName:      a.DetectorName,
			DetectorComponent: detectorComponentMap[a.DetectorName],
			SourceSeriesID:    string(seriesID),
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
		ID:        string(seriesID),
		Namespace: series.Namespace,
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
		resp.Points[i] = pointOutput{
			Timestamp: p.Timestamp,
			Value:     value,
		}
	}

	api.writeJSON(w, resp)
}

// handleAnomalies returns all detected anomalies.
func (api *TestBenchAPI) handleAnomalies(w http.ResponseWriter, r *http.Request) {
	// Check for detector filter
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
	storage := api.tb.GetStorage()

	toResponse := func(a observerdef.Anomaly) anomalyResponse {
		sourceSeriesID := string(a.SourceSeriesID)
		if storage != nil {
			sourceSeriesID = storage.CompactSeriesID(sourceSeriesID)
		}
		resp := anomalyResponse{
			Source:            string(a.Source),
			SourceSeriesID:    sourceSeriesID,
			DetectorName:      a.DetectorName,
			DetectorComponent: detectorComponentMap[a.DetectorName],
			Title:             a.Title,
			Description:       a.Description,
			Tags:              a.Tags,
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
		// Return only anomalies from specified detector
		byDetector := api.tb.GetMetricsAnomaliesByDetector()
		if anomalies, ok := byDetector[detectorFilter]; ok {
			for _, a := range anomalies {
				if a.DetectorName == "" || a.Timestamp == 0 {
					log.Printf("skipping malformed anomaly response: detector=%q source=%q ts=%d",
						a.DetectorName, a.Source, a.Timestamp)
					continue
				}
				response = append(response, toResponse(a))
			}
		}
	} else {
		// Return all anomalies
		anomalies := api.tb.GetMetricsAnomalies()
		for _, a := range anomalies {
			if a.DetectorName == "" || a.Timestamp == 0 {
				log.Printf("skipping malformed anomaly response: detector=%q source=%q ts=%d",
					a.DetectorName, a.Source, a.Timestamp)
				continue
			}
			response = append(response, toResponse(a))
		}
	}

	api.writeJSON(w, response)
}

// handleLogAnomalies returns anomalies emitted directly by log detectors.
func (api *TestBenchAPI) handleLogAnomalies(w http.ResponseWriter, r *http.Request) {
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
			Source:       string(a.Source),
			DetectorName: a.DetectorName,
			Title:        a.Title,
			Description:  a.Description,
			Tags:         a.Tags,
			Timestamp:    a.Timestamp,
			Score:        a.Score,
		})
	}

	api.writeJSON(w, response)
}

// handleLogs returns raw log entries with server-side filtering and pagination.
func (api *TestBenchAPI) handleLogs(w http.ResponseWriter, r *http.Request) {
	query := parseLogsQuery(r.URL.Query())

	type logEntryResponse struct {
		TimestampMs int64    `json:"timestampMs"`
		Status      string   `json:"status"`
		Content     string   `json:"content"`
		Hostname    string   `json:"hostname"`
		Tags        []string `json:"tags"`
	}

	logs := api.tb.GetRawLogs()

	// First pass: count total matches and collect the paginated window.
	total := 0
	result := make([]logEntryResponse, 0, query.limit)
	for _, l := range logs {
		if matchesLogsQuery(l, query) {
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
	}

	api.writeJSON(w, map[string]interface{}{
		"logs":   result,
		"total":  total,
		"limit":  query.limit,
		"offset": query.offset,
	})
}

// handleLogsSummary returns lightweight summary data about logs without bodies.
func (api *TestBenchAPI) handleLogsSummary(w http.ResponseWriter, r *http.Request) {
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

	// Build histogram with ~100 buckets.
	const numBuckets = 100
	type histBucket struct {
		TimestampMs int64 `json:"timestampMs"`
		Count       int   `json:"count"`
	}

	histogram := make([]histBucket, 0)
	if totalCount > 0 && maxTs > minTs {
		bucketWidth := (maxTs - minTs + numBuckets) / numBuckets // ceil division
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
		// All logs have the same timestamp — single bucket.
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
func (api *TestBenchAPI) handleCorrelations(w http.ResponseWriter, r *http.Request) {
	correlations := api.tb.GetCorrelations()
	storage := api.tb.GetStorage()

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
			tags := a.Tags
			if tags == nil {
				tags = []string{}
			}
			anomalies[j] = anomalyOutput{
				Source:      string(a.Source),
				Title:       a.Title,
				Description: a.Description,
				Timestamp:   a.Timestamp,
				Score:       a.Score,
				Tags:        tags,
			}
		}
		memberIDs := seriesIDsToStrings(c.MemberSeriesIDs)
		if storage != nil {
			for k, id := range memberIDs {
				memberIDs[k] = storage.CompactSeriesID(id)
			}
		}
		response[i] = correlationResponse{
			Pattern:         c.Pattern,
			Title:           c.Title,
			MemberSeriesIDs: memberIDs,
			MetricNames:     metricNamesToStrings(c.MetricNames),
			Anomalies:       anomalies,
			FirstSeen:       c.FirstSeen,
			LastUpdated:     c.LastUpdated,
		}
	}

	api.writeJSON(w, response)
}

func metricNamesToStrings(names []observerdef.MetricName) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = string(n)
	}
	return out
}

func seriesIDsToStrings(ids []observerdef.SeriesID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

// handleLeadLag returns lead-lag edges.
func (api *TestBenchAPI) handleLeadLag(w http.ResponseWriter, r *http.Request) {
	edges, enabled := api.tb.GetLeadLagEdges()
	if edges == nil {
		edges = []LeadLagEdge{}
	}
	api.writeJSON(w, map[string]interface{}{
		"enabled": enabled,
		"edges":   edges,
	})
}

// handleSurprise returns surprise edges.
func (api *TestBenchAPI) handleSurprise(w http.ResponseWriter, r *http.Request) {
	edges, enabled := api.tb.GetSurpriseEdges()
	if edges == nil {
		edges = []SurpriseEdge{}
	}
	api.writeJSON(w, map[string]interface{}{
		"enabled": enabled,
		"edges":   edges,
	})
}

// handleStats returns correlator statistics.
func (api *TestBenchAPI) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := api.tb.GetCorrelatorStats()
	api.writeJSON(w, stats)
}

// handleComponentAction handles /api/components/{name}/{action} (toggle, data).
func (api *TestBenchAPI) handleComponentAction(w http.ResponseWriter, r *http.Request) {
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
func (api *TestBenchAPI) handleCompressedCorrelations(w http.ResponseWriter, r *http.Request) {
	threshold := 0.75
	if t := r.URL.Query().Get("threshold"); t != "" {
		if parsed, err := strconv.ParseFloat(t, 64); err == nil && parsed > 0 && parsed <= 1 {
			threshold = parsed
		}
	}
	groups := cloneCompressedGroups(api.tb.GetCompressedCorrelations(threshold))
	// Translate MemberSources from full keys to compact numeric IDs.
	if storage := api.tb.GetStorage(); storage != nil {
		for i := range groups {
			for j, src := range groups[i].MemberSources {
				groups[i].MemberSources[j] = storage.CompactSeriesID(src)
			}
		}
	}
	api.writeJSON(w, groups)
}

// handleScore returns the Gaussian F1 score for the current analysis.
func (api *TestBenchAPI) handleScore(w http.ResponseWriter, r *http.Request) {
	sigma := 30.0
	if s := r.URL.Query().Get("sigma"); s != "" {
		if parsed, err := strconv.ParseFloat(s, 64); err == nil && parsed > 0 {
			sigma = parsed
		}
	}

	result, err := api.tb.ScoreCurrentAnalysis(sigma)
	if err != nil {
		api.writeJSON(w, map[string]interface{}{
			"available": false,
			"reason":    err.Error(),
		})
		return
	}

	api.writeJSON(w, map[string]interface{}{
		"available": true,
		"score":     result,
	})
}

// writeJSON writes a JSON response.
func (api *TestBenchAPI) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
		http.Error(w, `{"error":"encoding error"}`, http.StatusInternalServerError)
	}
}

// writeError writes an error response.
func (api *TestBenchAPI) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
