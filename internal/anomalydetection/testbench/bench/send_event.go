// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// sendReportedEventViaAPI posts a single ReportedEvent to the Datadog Events v2 API.
// It reads observer.event_reporter.sending_enabled from cfg; when false, it dry-runs
// (logs to stdout without calling the API).
// extraTags are appended to the event's existing tags.
func sendReportedEventViaAPI(cfg config.Component, logger log.Component, _ observerimpl.StateView, event ReportedEvent, extraTags []string) error {
	allTags := append(append([]string{}, event.Tags...), extraTags...)
	ts := time.Unix(event.FirstSeen, 0).UTC().Format(time.RFC3339)
	aggKey := "observer:" + event.Pattern

	if !cfg.GetBool("observer.event_reporter.sending_enabled") {
		fmt.Printf("[dry-run] bench event: pattern=%s title=%q aggKey=%s timestamp=%s\n%s\n\n",
			event.Pattern, event.Title, aggKey, ts, event.Message)
		return nil
	}

	apiKey := cfg.GetString("api_key")
	if apiKey == "" {
		return errors.New("api_key is not set in configuration")
	}

	ctx := context.WithValue(
		datadog.NewDefaultContext(context.Background()),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{"apiKeyAuth": {Key: apiKey}},
	)
	eventsAPI := datadogV2.NewEventsApi(datadog.NewAPIClient(datadog.NewConfiguration()))

	payload := datadogV2.EventCreateRequestPayload{
		Data: datadogV2.EventCreateRequest{
			Type: datadogV2.EVENTCREATEREQUESTTYPE_EVENT,
			Attributes: datadogV2.EventPayload{
				Title:          event.Title,
				Message:        datadog.PtrString(event.Message),
				Category:       datadogV2.EVENTCATEGORY_CHANGE,
				Tags:           allTags,
				Timestamp:      datadog.PtrString(ts),
				AggregationKey: datadog.PtrString(aggKey),
			},
		},
	}

	logger.Infof("[bench] sending change event: pattern=%s title=%q\n", event.Pattern, event.Title)

	_, httpResp, err := eventsAPI.CreateEvent(ctx, payload)
	if err != nil && httpResp != nil {
		body, readErr := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if readErr == nil {
			return fmt.Errorf("API error (HTTP %d): %s", httpResp.StatusCode, string(body))
		}
	}
	return err
}
