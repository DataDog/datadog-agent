// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/mcp/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultDuration = 10  // seconds
	maxDuration     = 60  // seconds
	defaultLimit    = 100 // messages
	maxLimit        = 500 // messages
)

// LogsHandler implements the logs monitoring MCP tool
type LogsHandler struct {
	logsAgent logsAgent.Component
	config    pkgconfigmodel.Reader
	mu        sync.Mutex
}

// NewLogsHandler creates a new logs tool handler
func NewLogsHandler(logsAgent logsAgent.Component, cfg pkgconfigmodel.Reader) (*LogsHandler, error) {
	if logsAgent == nil {
		return nil, fmt.Errorf("logs agent component is required")
	}

	return &LogsHandler{
		logsAgent: logsAgent,
		config:    cfg,
	}, nil
}

// Handle processes GetLogs requests
func (h *LogsHandler) Handle(ctx context.Context, req *types.ToolRequest) (*types.ToolResponse, error) {
	// Parse request parameters
	params, err := h.parseLogsParams(req.Parameters)
	if err != nil {
		return &types.ToolResponse{
			ToolName:  req.ToolName,
			Error:     fmt.Sprintf("invalid parameters: %v", err),
			RequestID: req.RequestID,
		}, fmt.Errorf("invalid parameters: %w", err)
	}

	// Get logs snapshot
	snapshot, err := h.getLogs(ctx, params)
	if err != nil {
		return &types.ToolResponse{
			ToolName:  req.ToolName,
			Error:     fmt.Sprintf("failed to get logs: %v", err),
			RequestID: req.RequestID,
		}, fmt.Errorf("failed to get logs: %w", err)
	}

	return &types.ToolResponse{
		ToolName:  req.ToolName,
		Result:    snapshot,
		RequestID: req.RequestID,
	}, nil
}

func (h *LogsHandler) parseLogsParams(params map[string]interface{}) (*LogsParams, error) {
	result := &LogsParams{
		Duration: defaultDuration,
		Limit:    defaultLimit,
	}

	// Parse query (regex pattern)
	if query, ok := params["query"]; ok {
		if str, ok := query.(string); ok {
			result.Query = str
			// Validate regex
			if _, err := regexp.Compile(str); err != nil {
				return nil, fmt.Errorf("invalid regex pattern: %w", err)
			}
		}
	}

	// Parse source filter
	if source, ok := params["source"]; ok {
		if str, ok := source.(string); ok {
			result.Source = str
		}
	}

	// Parse service filter
	if service, ok := params["service"]; ok {
		if str, ok := service.(string); ok {
			result.Service = str
		}
	}

	// Parse name filter
	if name, ok := params["name"]; ok {
		if str, ok := name.(string); ok {
			result.Name = str
		}
	}

	// Parse type filter
	if typ, ok := params["type"]; ok {
		if str, ok := typ.(string); ok {
			result.Type = str
		}
	}

	// Parse duration
	if duration, ok := params["duration"]; ok {
		switch v := duration.(type) {
		case float64:
			result.Duration = int(v)
		case int:
			result.Duration = v
		}
	}

	// Parse limit
	if limit, ok := params["limit"]; ok {
		switch v := limit.(type) {
		case float64:
			result.Limit = int(v)
		case int:
			result.Limit = v
		}
	}

	// Apply config-based max limits
	maxLimitConfig := h.config.GetInt("mcp.tools.logs.max_logs_per_request")
	if maxLimitConfig == 0 {
		maxLimitConfig = maxLimit
	}
	if result.Limit > maxLimitConfig {
		result.Limit = maxLimitConfig
	}
	if result.Limit <= 0 {
		result.Limit = defaultLimit
	}

	maxDurationConfig := h.config.GetInt("mcp.tools.logs.max_duration")
	if maxDurationConfig == 0 {
		maxDurationConfig = maxDuration
	}
	if result.Duration > maxDurationConfig {
		result.Duration = maxDurationConfig
	}
	if result.Duration <= 0 {
		result.Duration = defaultDuration
	}

	return result, nil
}

func (h *LogsHandler) getLogs(ctx context.Context, params *LogsParams) (*LogsSnapshot, error) {
	log.Infof("GetLogs: starting, duration=%d limit=%d query=%s", params.Duration, params.Limit, params.Query)

	// Ensure only one collection at a time
	log.Debug("GetLogs: acquiring lock")
	h.mu.Lock()
	defer h.mu.Unlock()
	log.Debug("GetLogs: lock acquired")

	receiver := h.logsAgent.GetMessageReceiver()
	if receiver == nil {
		return nil, fmt.Errorf("logs message receiver not available")
	}
	log.Debug("GetLogs: got message receiver")

	startTime := time.Now()
	duration := time.Duration(params.Duration) * time.Second

	// Enable the receiver to start collecting messages
	wasEnabled := receiver.IsEnabled()
	log.Debugf("GetLogs: receiver wasEnabled=%v", wasEnabled)
	if !wasEnabled {
		receiver.SetEnabled(true)
		log.Debug("GetLogs: enabled receiver")
	}

	// Create diagnostic filters from params
	filters := &diagnostic.Filters{
		Name:    params.Name,
		Type:    params.Type,
		Source:  params.Source,
		Service: params.Service,
	}

	// Compile regex if provided
	var queryRegex *regexp.Regexp
	if params.Query != "" {
		var err error
		queryRegex, err = regexp.Compile(params.Query)
		if err != nil {
			return nil, fmt.Errorf("invalid query regex: %w", err)
		}
	}

	// Create done channel to signal the filter goroutine to stop
	done := make(chan struct{})

	// Start filtering - this returns a channel we'll read from
	log.Debug("GetLogs: calling receiver.Filter")
	logChan := receiver.Filter(filters, done)
	log.Debug("GetLogs: got logChan, starting collection loop")

	// Collect logs
	var entries []*LogEntry
	totalCollected := 0

	// Create a timer for the collection duration
	timer := time.NewTimer(duration)
	defer timer.Stop()

	// Also respect context cancellation
	ctxDone := ctx.Done()

	// Collection loop with explicit timeout handling
	collecting := true
	loopCount := 0
	for collecting {
		loopCount++
		select {
		case logStr, ok := <-logChan:
			if !ok {
				log.Debug("GetLogs: logChan closed")
				collecting = false
				continue
			}

			totalCollected++
			log.Debugf("GetLogs: received log %d", totalCollected)

			// Apply regex filter on the raw message
			if queryRegex != nil && !queryRegex.MatchString(logStr) {
				continue
			}

			// Parse the formatted log string into a LogEntry
			entry := parseLogEntry(logStr, startTime)
			entries = append(entries, entry)

			// Check if we've hit the limit
			if len(entries) >= params.Limit {
				log.Debugf("GetLogs: hit limit %d", params.Limit)
				collecting = false
			}

		case <-timer.C:
			log.Debugf("GetLogs: timeout after %v, collected %d logs", duration, totalCollected)
			collecting = false

		case <-ctxDone:
			log.Debug("GetLogs: context cancelled")
			collecting = false
		}
	}

	log.Debugf("GetLogs: collection loop done after %d iterations, closing done channel", loopCount)

	// Signal the filter goroutine to stop FIRST
	close(done)

	// Give the filter goroutine a moment to clean up
	// and drain any remaining items from logChan
	log.Debug("GetLogs: draining logChan")
	drainCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
drainLoop:
	for {
		select {
		case _, ok := <-logChan:
			if !ok {
				break drainLoop
			}
			// Discard remaining messages
		case <-drainCtx.Done():
			break drainLoop
		}
	}
	log.Debug("GetLogs: drain complete")

	// Now safe to disable receiver if we enabled it
	if !wasEnabled {
		receiver.SetEnabled(false)
		log.Debug("GetLogs: disabled receiver")
	}

	snapshot := &LogsSnapshot{
		Timestamp:      startTime.Unix(),
		Duration:       params.Duration,
		Messages:       entries,
		TotalCollected: totalCollected,
		Count:          len(entries),
		Query:          params.Query,
		Truncated:      len(entries) >= params.Limit && totalCollected > len(entries),
	}

	log.Infof("GetLogs: complete, returning %d logs", len(entries))
	return snapshot, nil
}

// parseLogEntry parses a formatted log string into a LogEntry
// Format: "Integration Name: X | Type: Y | Status: Z | Timestamp: T | Hostname: H | Service: S | Source: O | Tags: T | Message: M"
func parseLogEntry(logStr string, collectTime time.Time) *LogEntry {
	entry := &LogEntry{
		Timestamp: collectTime.UnixNano(),
		Message:   logStr, // Default to full string if parsing fails
	}

	// Try to extract fields from the formatted string
	// The format is: "Integration Name: X | Type: Y | Status: Z | ..."
	fields := extractFields(logStr)

	if name, ok := fields["Integration Name"]; ok {
		entry.Name = name
	}
	if typ, ok := fields["Type"]; ok {
		entry.Type = typ
	}
	if status, ok := fields["Status"]; ok {
		entry.Status = status
	}
	if hostname, ok := fields["Hostname"]; ok {
		entry.Hostname = hostname
	}
	if service, ok := fields["Service"]; ok {
		entry.Service = service
	}
	if source, ok := fields["Source"]; ok {
		entry.Source = source
	}
	if msg, ok := fields["Message"]; ok {
		entry.Message = msg
	}
	if tags, ok := fields["Tags"]; ok && tags != "" && tags != "[]" {
		entry.Tags = []string{tags}
	}

	return entry
}

// extractFields parses a pipe-delimited log format into a map
func extractFields(logStr string) map[string]string {
	fields := make(map[string]string)
	parts := splitPipe(logStr)

	for _, part := range parts {
		// Find the colon separator
		for i := 0; i < len(part); i++ {
			if part[i] == ':' {
				key := trimSpace(part[:i])
				value := trimSpace(part[i+1:])
				fields[key] = value
				break
			}
		}
	}

	return fields
}

// splitPipe splits a string by " | " delimiter
func splitPipe(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s)-2; i++ {
		if s[i] == ' ' && s[i+1] == '|' && s[i+2] == ' ' {
			parts = append(parts, s[start:i])
			start = i + 3
			i += 2
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// trimSpace removes leading and trailing whitespace
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

// Close releases resources held by the handler
func (h *LogsHandler) Close() {
	log.Debug("Logs handler closed")
}

// mcpFormatter is a custom formatter for MCP that preserves message structure
type mcpFormatter struct{}

func (f *mcpFormatter) Format(m *message.Message, eventType string, rendered []byte) string {
	// Return the raw content for regex matching
	return string(rendered)
}

// SourcesHandler implements the log sources listing MCP tool
type SourcesHandler struct {
	logsAgent logsAgent.Component
	config    pkgconfigmodel.Reader
}

// NewSourcesHandler creates a new log sources listing handler
func NewSourcesHandler(logsAgent logsAgent.Component, cfg pkgconfigmodel.Reader) (*SourcesHandler, error) {
	if logsAgent == nil {
		return nil, fmt.Errorf("logs agent component is required")
	}

	return &SourcesHandler{
		logsAgent: logsAgent,
		config:    cfg,
	}, nil
}

// Handle processes ListLogSources requests
func (h *SourcesHandler) Handle(ctx context.Context, req *types.ToolRequest) (*types.ToolResponse, error) {
	// Parse request parameters
	params := h.parseSourcesParams(req.Parameters)

	// Get log sources list
	sourcesList, err := h.listSources(ctx, params)
	if err != nil {
		return &types.ToolResponse{
			ToolName:  req.ToolName,
			Error:     fmt.Sprintf("failed to list log sources: %v", err),
			RequestID: req.RequestID,
		}, fmt.Errorf("failed to list log sources: %w", err)
	}

	return &types.ToolResponse{
		ToolName:  req.ToolName,
		Result:    sourcesList,
		RequestID: req.RequestID,
	}, nil
}

func (h *SourcesHandler) parseSourcesParams(params map[string]interface{}) *ListSourcesParams {
	result := &ListSourcesParams{}

	// Parse type filter
	if typ, ok := params["type"]; ok {
		if str, ok := typ.(string); ok {
			result.Type = str
		}
	}

	// Parse status filter
	if status, ok := params["status"]; ok {
		if str, ok := status.(string); ok {
			result.Status = str
		}
	}

	return result
}

func (h *SourcesHandler) listSources(ctx context.Context, params *ListSourcesParams) (*LogSourcesList, error) {
	log.Infof("ListLogSources: starting, type=%s status=%s", params.Type, params.Status)

	log.Debug("ListLogSources: getting sources")
	sources := h.logsAgent.GetSources()
	if sources == nil {
		return nil, fmt.Errorf("log sources not available")
	}

	log.Debug("ListLogSources: calling GetSources()")
	allSources := sources.GetSources()
	log.Debugf("ListLogSources: got %d sources", len(allSources))

	now := time.Now()

	var sourceInfos []*LogSourceInfo
	typeCounts := make(map[string]int)

	for i, source := range allSources {
		log.Debugf("ListLogSources: processing source %d", i)

		// Skip hidden sources
		if source == nil || source.Config == nil {
			log.Debugf("ListLogSources: skipping nil source %d", i)
			continue
		}

		sourceType := source.Config.Type
		statusStr := ""
		if source.Status != nil {
			statusStr = source.Status.String()
		}

		// Apply type filter
		if params.Type != "" && sourceType != params.Type {
			continue
		}

		// Apply status filter
		if params.Status != "" && statusStr != params.Status {
			continue
		}

		// Count by type
		typeCounts[sourceType]++

		log.Debugf("ListLogSources: building info for source %d (name=%s, type=%s)", i, source.Name, sourceType)

		// Build source info
		info := &LogSourceInfo{
			Name:   source.Name,
			Type:   sourceType,
			Status: statusStr,
			Inputs: source.GetInputs(),
			Config: &LogSourceConfig{
				Path:       source.Config.Path,
				Service:    source.Config.Service,
				Source:     source.Config.Source,
				Identifier: source.Config.Identifier,
			},
		}

		// Add tags if present
		if len(source.Config.Tags) > 0 {
			info.Config.Tags = source.Config.Tags
		}

		// Add bytes read if available
		if source.BytesRead != nil {
			info.BytesRead = source.BytesRead.Get()
		}

		// Add additional info from status
		if source.Status != nil {
			log.Debugf("ListLogSources: getting info status for source %d", i)
			infoMap := source.GetInfoStatus()
			if len(infoMap) > 0 {
				info.Info = make(map[string]string)
				for k, v := range infoMap {
					if len(v) > 0 {
						info.Info[k] = v[0]
					}
				}
			}
		}

		sourceInfos = append(sourceInfos, info)
	}

	log.Infof("ListLogSources: complete, returning %d sources", len(sourceInfos))

	return &LogSourcesList{
		Timestamp:  now.Unix(),
		Sources:    sourceInfos,
		Count:      len(sourceInfos),
		TypeCounts: typeCounts,
	}, nil
}

// Close releases resources held by the handler
func (h *SourcesHandler) Close() {
	log.Debug("Sources handler closed")
}
