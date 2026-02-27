// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"net/url"
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
	mux.HandleFunc("/api/status", api.cors(api.handleStatus))
	mux.HandleFunc("/api/scenarios", api.cors(api.handleScenarios))
	mux.HandleFunc("/api/scenarios/", api.cors(api.handleScenarioAction))
	mux.HandleFunc("/api/components", api.cors(api.handleComponents))
	mux.HandleFunc("/api/series", api.cors(api.handleSeriesList))
	mux.HandleFunc("/api/series/id/", api.cors(api.handleSeriesDataByID))
	mux.HandleFunc("/api/series/", api.cors(api.handleSeriesData))
	mux.HandleFunc("/api/anomalies", api.cors(api.handleAnomalies))
	mux.HandleFunc("/api/log-anomalies", api.cors(api.handleLogAnomalies))
	mux.HandleFunc("/api/correlations", api.cors(api.handleCorrelations))
	mux.HandleFunc("/api/leadlag", api.cors(api.handleLeadLag))
	mux.HandleFunc("/api/surprise", api.cors(api.handleSurprise))
	mux.HandleFunc("/api/graphsketch", api.cors(api.handleGraphSketch))
	mux.HandleFunc("/api/stats", api.cors(api.handleStats))
	mux.HandleFunc("/api/config", api.cors(api.handleConfigUpdate))
	mux.HandleFunc("/api/components/", api.cors(api.handleComponentAction))
	mux.HandleFunc("/api/correlators/", api.cors(api.handleCorrelatorData))
	mux.HandleFunc("/api/correlations/compressed", api.cors(api.handleCompressedCorrelations))

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

// cors wraps a handler with CORS headers.
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

	// Get series from all namespaces with both aggregations
	for _, ns := range storage.Namespaces() {
		for _, agg := range []Aggregate{AggregateAverage, AggregateCount} {
			series := storage.AllSeries(ns, agg)
			for _, s := range series {
				nameWithAgg := s.Name + ":" + aggSuffix(agg)
				allSeries = append(allSeries, seriesInfo{
					ID:         seriesKey(s.Namespace, nameWithAgg, s.Tags),
					Namespace:  s.Namespace,
					Name:       nameWithAgg,
					Tags:       s.Tags,
					PointCount: len(s.Points),
				})
			}
		}
	}

	api.writeJSON(w, allSeries)
}

// handleSeriesDataByID returns data for a specific series by canonical id.
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
	namespace, nameWithAgg, tags, ok := parseSeriesKey(seriesID)
	if !ok {
		api.writeError(w, http.StatusBadRequest, "invalid series id")
		return
	}
	api.handleSeriesDataForSeries(w, namespace, nameWithAgg, tags, observerdef.SeriesID(seriesID))
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
		AnalyzerName      string `json:"analyzerName"`
		AnalyzerComponent string `json:"analyzerComponent"`
		SourceSeriesID    string `json:"sourceSeriesId"`
		Title             string `json:"title"`
	}

	var markers []anomalyMarker
	analyzerComponentMap := api.tb.GetAnalyzerComponentMap()
	for _, a := range anomalies {
		if a.AnalyzerName == "" || a.Timestamp == 0 {
			log.Printf("skipping malformed anomaly marker for series %q: analyzer=%q ts=%d",
				string(seriesID), a.AnalyzerName, a.Timestamp)
			continue
		}
		markers = append(markers, anomalyMarker{
			Timestamp:         a.Timestamp,
			AnalyzerName:      a.AnalyzerName,
			AnalyzerComponent: analyzerComponentMap[a.AnalyzerName],
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
	// Check for analyzer filter
	analyzerFilter := r.URL.Query().Get("analyzer")

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
		AnalyzerName      string             `json:"analyzerName"`
		AnalyzerComponent string             `json:"analyzerComponent"`
		Title             string             `json:"title"`
		Description       string             `json:"description"`
		Tags              []string           `json:"tags"`
		Timestamp         int64              `json:"timestamp"`
		DebugInfo         *debugInfoResponse `json:"debugInfo,omitempty"`
	}

	analyzerComponentMap := api.tb.GetAnalyzerComponentMap()

	toResponse := func(a observerdef.AnomalyOutput) anomalyResponse {
		resp := anomalyResponse{
			Source:            string(a.Source),
			SourceSeriesID:    string(a.SourceSeriesID),
			AnalyzerName:      a.AnalyzerName,
			AnalyzerComponent: analyzerComponentMap[a.AnalyzerName],
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

	if analyzerFilter != "" {
		// Return only anomalies from specified analyzer
		byAnalyzer := api.tb.GetMetricsAnomaliesByAnalyzer()
		if anomalies, ok := byAnalyzer[analyzerFilter]; ok {
			for _, a := range anomalies {
				if a.AnalyzerName == "" || a.Timestamp == 0 {
					log.Printf("skipping malformed anomaly response: analyzer=%q source=%q ts=%d",
						a.AnalyzerName, a.Source, a.Timestamp)
					continue
				}
				response = append(response, toResponse(a))
			}
		}
	} else {
		// Return all anomalies
		anomalies := api.tb.GetMetricsAnomalies()
		for _, a := range anomalies {
			if a.AnalyzerName == "" || a.Timestamp == 0 {
				log.Printf("skipping malformed anomaly response: analyzer=%q source=%q ts=%d",
					a.AnalyzerName, a.Source, a.Timestamp)
				continue
			}
			response = append(response, toResponse(a))
		}
	}

	api.writeJSON(w, response)
}

// handleLogAnomalies returns anomalies emitted directly by log processors.
func (api *TestBenchAPI) handleLogAnomalies(w http.ResponseWriter, r *http.Request) {
	processorFilter := r.URL.Query().Get("processor")

	type logAnomalyResponse struct {
		Source        string   `json:"source"`
		ProcessorName string   `json:"processorName"`
		Title         string   `json:"title"`
		Description   string   `json:"description"`
		Tags          []string `json:"tags"`
		Timestamp     int64    `json:"timestamp"`
		Score         *float64 `json:"score,omitempty"`
	}

	var anomalies []observerdef.AnomalyOutput
	if processorFilter != "" {
		byProcessor := api.tb.GetLogAnomaliesByProcessor()
		anomalies = byProcessor[processorFilter]
	} else {
		anomalies = api.tb.GetLogAnomalies()
	}

	response := make([]logAnomalyResponse, 0, len(anomalies))
	for _, a := range anomalies {
		response = append(response, logAnomalyResponse{
			Source:        string(a.Source),
			ProcessorName: a.AnalyzerName,
			Title:         a.Title,
			Description:   a.Description,
			Tags:          a.Tags,
			Timestamp:     a.Timestamp,
			Score:         a.Score,
		})
	}

	api.writeJSON(w, response)
}

// handleCorrelations returns detected correlations.
func (api *TestBenchAPI) handleCorrelations(w http.ResponseWriter, r *http.Request) {
	correlations := api.tb.GetCorrelations()

	type anomalyOutput struct {
		Source      string   `json:"source"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Timestamp   int64    `json:"timestamp"`
		Score       *float64 `json:"score,omitempty"`
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
			anomalies[j] = anomalyOutput{
				Source:      string(a.Source),
				Title:       a.Title,
				Description: a.Description,
				Timestamp:   a.Timestamp,
				Score:       a.Score,
			}
		}
		response[i] = correlationResponse{
			Pattern:         c.Pattern,
			Title:           c.Title,
			MemberSeriesIDs: seriesIDsToStrings(c.MemberSeriesIDs),
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

// handleGraphSketch returns graph sketch edges.
func (api *TestBenchAPI) handleGraphSketch(w http.ResponseWriter, r *http.Request) {
	edges, enabled := api.tb.GetGraphSketchEdges()
	if edges == nil {
		edges = []EdgeInfo{}
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

// handleComponentAction handles POST /api/components/{name}/toggle.
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
	default:
		api.writeError(w, http.StatusBadRequest, "unknown action: "+action)
	}
}

// handleCorrelatorData handles GET /api/correlators/{name}.
func (api *TestBenchAPI) handleCorrelatorData(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/correlators/")
	data, enabled := api.tb.GetCorrelatorData(name)
	api.writeJSON(w, map[string]interface{}{
		"enabled": enabled,
		"data":    data,
	})
}

// handleCompressedCorrelations returns compressed group descriptions.
func (api *TestBenchAPI) handleCompressedCorrelations(w http.ResponseWriter, r *http.Request) {
	threshold := 0.75
	if t := r.URL.Query().Get("threshold"); t != "" {
		if parsed, err := strconv.ParseFloat(t, 64); err == nil && parsed > 0 && parsed <= 1 {
			threshold = parsed
		}
	}
	groups := api.tb.GetCompressedCorrelations(threshold)
	api.writeJSON(w, groups)
}

// handleConfigUpdate handles POST /api/config to update server configuration.
func (api *TestBenchAPI) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		api.writeError(w, http.StatusMethodNotAllowed, "use POST to update config")
		return
	}

	var req ConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := api.tb.UpdateConfigAndReanalyze(req); err != nil {
		api.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return updated status
	api.writeJSON(w, api.tb.GetStatus())
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
