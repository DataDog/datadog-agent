// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// eventSender formats and dispatches one Datadog event per correlation.
// When api is nil, send prints to stdout (dry-run mode) instead of calling the API.
type eventSender struct {
	api *datadogV2.EventsApi
	ctx context.Context
}

// newEventSender creates an eventSender. When dryRun is true, api is left nil.
func newEventSender(dryRun bool) (*eventSender, error) {
	if dryRun {
		return &eventSender{}, nil
	}
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("DD_API_KEY environment variable is not set")
	}
	ctx := context.WithValue(
		datadog.NewDefaultContext(context.Background()),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{"apiKeyAuth": {Key: apiKey}},
	)
	return &eventSender{
		api: datadogV2.NewEventsApi(datadog.NewAPIClient(datadog.NewConfiguration())),
		ctx: ctx,
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
			metricLines = append(metricLines, "- "+a.Description)
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

	log.Printf("[observer] sending event: pattern=%s title=%q timestamp=%s\n%s\n", c.Pattern, c.Title, ts, text)

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
			log.Printf("[observer] API error response (HTTP %d): %s", httpResp.StatusCode, string(body))
		}
	}
	return err
}

// sendCorrelationEvents creates a sender and dispatches one event per correlation.
func sendCorrelationEvents(correlations []observerdef.ActiveCorrelation, dryRun bool) error {
	sender, err := newEventSender(dryRun)
	if err != nil {
		return err
	}
	for _, c := range correlations {
		if err := sender.send(c); err != nil {
			return fmt.Errorf("sending event for pattern %q: %w", c.Pattern, err)
		}
	}
	return nil
}
