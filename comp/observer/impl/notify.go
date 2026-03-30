// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// eventSender formats and dispatches one Datadog event per correlation.
// When api is nil, send prints to stdout (dry-run mode) instead of calling the API.
type eventSender struct {
	api    *datadogV2.EventsApi
	ctx    context.Context
	logger log.Component
}

// newEventSender creates an eventSender. It reads observer.event_reporter.sending_enabled
// from cfg; when false, api is left nil and events are only logged (dry-run mode).
func newEventSender(cfg config.Component, logger log.Component) (*eventSender, error) {
	if !cfg.GetBool("observer.event_reporter.sending_enabled") {
		return &eventSender{logger: logger}, nil
	}
	apiKey := cfg.GetString("api_key")
	if apiKey == "" {
		return nil, errors.New("api_key is not set in configuration")
	}
	ctx := context.WithValue(
		datadog.NewDefaultContext(context.Background()),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{"apiKeyAuth": {Key: apiKey}},
	)
	return &eventSender{
		api:    datadogV2.NewEventsApi(datadog.NewAPIClient(datadog.NewConfiguration())),
		ctx:    ctx,
		logger: logger,
	}, nil
}

// correlationMessage builds the event message body for a correlation.
func correlationMessage(c observerdef.ActiveCorrelation) string {
	var metricLines, logLines []string
	for _, a := range c.Anomalies {
		if a.Description == "" {
			continue
		}
		if a.Type == observerdef.AnomalyTypeLog {
			logLines = append(logLines, "- "+a.Description)
		} else {
			var pattern string
			if a.Context != nil {
				pattern = strings.TrimSpace(a.Context.Pattern)
			}
			// If this metric is a log related one, find its pattern and create a custom message
			// TODO(celian): When this will be split by tags, add tags to the description. Then be sure that we don't have twice (pattern, tags) tuples
			if a.Source.Namespace == "log_pattern_extractor" && pattern != "" {
				var example string
				if a.Context.Example != "" {
					example = "\tlog example: " + strings.TrimSpace(a.Context.Example)
				}
				var ratePart string
				if a.DebugInfo != nil {
					ratePart = fmt.Sprintf("\tcurrent rate: %.1flog/s", a.DebugInfo.CurrentValue)
				} else {
					ratePart = "\tcurrent rate: unknown"
				}
				logDescription := fmt.Sprintf("Log pattern change rate detected: %s%s%s", pattern, example, ratePart)
				logLines = append(logLines, "- "+logDescription)
			} else {
				metricLines = append(metricLines, "- "+a.Description)
			}
		}
	}
	var sections []string
	if len(metricLines) > 0 {
		sections = append(sections, fmt.Sprintf("Metric anomalies (%d):\n%s", len(metricLines), strings.Join(metricLines, "\n")))
	}
	if len(logLines) > 0 {
		sections = append(sections, fmt.Sprintf("Log anomalies (%d):\n%s", len(logLines), strings.Join(logLines, "\n")))
	}
	const maxLen = 4000
	text := "The following anomalies were detected and are likely related:\n\n" + strings.Join(sections, "\n\n")
	if len(text) > maxLen {
		text = text[:maxLen-3] + "..."
	}
	return text
}

// send formats a correlation into an event and either prints or posts it.
func (s *eventSender) send(c observerdef.ActiveCorrelation) error {
	text := correlationMessage(c)
	ts := time.Unix(c.FirstSeen, 0).UTC().Format(time.RFC3339)

	s.logger.Infof("[observer] sending event: pattern=%s title=%q timestamp=%s\n%s\n", c.Pattern, c.Title, ts, text)

	if s.api == nil {
		fmt.Printf("[dry-run] pattern=%s title=%q timestamp=%s\n%s\n\n", c.Pattern, c.Title, ts, text)
		return nil
	}

	attrs := datadogV2.AlertEventCustomAttributesAsEventPayloadAttributes(
		datadogV2.NewAlertEventCustomAttributes(datadogV2.ALERTEVENTCUSTOMATTRIBUTESSTATUS_ERROR),
	)
	payload := datadogV2.EventCreateRequestPayload{
		Data: datadogV2.EventCreateRequest{
			Type: datadogV2.EVENTCREATEREQUESTTYPE_EVENT,
			Attributes: datadogV2.EventPayload{
				Title:      c.Title,
				Message:    datadog.PtrString(text),
				Category:   datadogV2.EVENTCATEGORY_ALERT,
				Tags:       []string{"source:agent-q-branch-observer", "pattern:" + c.Pattern},
				Attributes: attrs,
			},
		},
	}
	_, httpResp, err := s.api.CreateEvent(s.ctx, payload)
	if err != nil && httpResp != nil {
		body, readErr := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if readErr == nil {
			return fmt.Errorf("API error (HTTP %d): %s", httpResp.StatusCode, string(body))
		}
	}
	return err
}

// sendCorrelationEvents sends one event per correlation.
func (s *eventSender) sendCorrelationEvents(correlations []observerdef.ActiveCorrelation) {
	for _, c := range correlations {
		if err := s.send(c); err != nil {
			s.logger.Errorf("[observer] failed to send event for pattern %s: %v", c.Pattern, err)
		}
	}
}
