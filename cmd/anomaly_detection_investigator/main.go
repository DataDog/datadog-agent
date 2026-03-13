// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

const filterSource = "source:agent-q-branch-observer"

func main() {
	interval := flag.Int("interval", 5*60, "seconds between API polls; also the idle window (display+reset when a poll returns no new events)")
	maxDuration := flag.Int("max-duration", 14*60, "max seconds to accumulate before forced display+reset")
	runBits := flag.Bool("run_bits", false, "trigger Bits AI investigations when a batch is ready (default: display only)")
	fromStr := flag.String("from", "", "RFC3339 start time for one-shot historical fetch (e.g. 2026-03-13T10:00:00Z)")
	toStr := flag.String("to", "", "RFC3339 end time for one-shot historical fetch (e.g. 2026-03-13T11:00:00Z)")
	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "error: unrecognized arguments: %s\n", strings.Join(flag.Args(), " "))
		flag.Usage()
		os.Exit(1)
	}

	apiKey := os.Getenv("DD_API_KEY")
	appKey := os.Getenv("DD_APP_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: DD_API_KEY environment variable is required")
		os.Exit(1)
	}
	if appKey == "" {
		fmt.Fprintln(os.Stderr, "error: DD_APP_KEY environment variable is required")
		os.Exit(1)
	}

	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {Key: apiKey},
			"appKeyAuth": {Key: appKey},
		},
	)

	cfg := datadog.NewConfiguration()
	eventsAPI := datadogV2.NewEventsApi(datadog.NewAPIClient(cfg))

	if *fromStr != "" || *toStr != "" {
		if *fromStr == "" || *toStr == "" {
			fmt.Fprintln(os.Stderr, "error: --from and --to must both be set for one-shot mode")
			os.Exit(1)
		}
		from, err := time.Parse(time.RFC3339, *fromStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid --from: %v\n", err)
			os.Exit(1)
		}
		to, err := time.Parse(time.RFC3339, *toStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid --to: %v\n", err)
			os.Exit(1)
		}
		runOnce(ctx, eventsAPI, apiKey, appKey, from, to, *runBits)
		return
	}

	fmt.Printf("Watching for events [%s]\n", filterSource)
	fmt.Printf("Interval: %ds | Max duration: %ds | Run Bits: %v\n\n", *interval, *maxDuration, *runBits)

	run(ctx, eventsAPI, apiKey, appKey,
		time.Duration(*interval)*time.Second,
		time.Duration(*maxDuration)*time.Second,
		*runBits,
	)
}

func runOnce(ctx context.Context, eventsAPI *datadogV2.EventsApi, apiKey, appKey string, from, to time.Time, runBits bool) {
	fmt.Printf("[one-shot] Fetching events from %s to %s\n", from.Format(time.RFC3339), to.Format(time.RFC3339))

	events, err := fetchEvents(ctx, eventsAPI, from, to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching events: %v\n", err)
		os.Exit(1)
	}
	if len(events) == 0 {
		fmt.Println("no events found")
		return
	}

	printEvents(events, "one-shot", !runBits)
	if err := triggerInvestigation(apiKey, appKey, events, from, !runBits); err != nil {
		fmt.Fprintf(os.Stderr, "investigation trigger error: %v\n", err)
	}
}

func run(ctx context.Context, eventsAPI *datadogV2.EventsApi, apiKey, appKey string, interval, maxDur time.Duration, runBits bool) {
	var (
		accumulated []datadogV2.EventResponse
		batchStart  time.Time
		lastQueried = time.Now()
		seenIDs     = make(map[string]struct{})
	)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		from := lastQueried
		to := time.Now()
		lastQueried = to

		events, err := fetchEvents(ctx, eventsAPI, from, to)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] poll error: %v\n", clock(), err)
			continue
		}

		newCount := 0
		for _, e := range events {
			if id := e.GetId(); id != "" {
				if _, seen := seenIDs[id]; !seen {
					seenIDs[id] = struct{}{}
					accumulated = append(accumulated, e)
					newCount++
				}
			}
		}

		if newCount > 0 {
			if batchStart.IsZero() {
				batchStart = time.Now()
			}
			fmt.Printf("[%s] +%d new event(s) | total: %d\n", clock(), newCount, len(accumulated))
		}

		if len(accumulated) == 0 {
			continue
		}

		elapsed := time.Since(batchStart)
		reason := ""
		switch {
		case elapsed >= maxDur:
			reason = fmt.Sprintf("max duration reached (%s)", elapsed.Round(time.Second))
		case newCount == 0:
			reason = "idle (no new events this poll)"
		}

		if reason != "" {
			printEvents(accumulated, reason, !runBits)
			if err := triggerInvestigation(apiKey, appKey, accumulated, batchStart, !runBits); err != nil {
				fmt.Fprintf(os.Stderr, "[%s] investigation trigger error: %v\n", clock(), err)
			}
			if runBits {
				accumulated = nil
				batchStart = time.Time{}
				seenIDs = make(map[string]struct{})
			}
		}
	}
}

func fetchEvents(ctx context.Context, api *datadogV2.EventsApi, from, to time.Time) ([]datadogV2.EventResponse, error) {
	params := datadogV2.NewListEventsOptionalParameters().
		WithFilterQuery(filterSource).
		WithFilterFrom(from.UTC().Format(time.RFC3339)).
		WithFilterTo(to.UTC().Format(time.RFC3339)).
		WithPageLimit(1000)

	var all []datadogV2.EventResponse
	ch, cancel := api.ListEventsWithPagination(ctx, *params)
	defer cancel()
	for result := range ch {
		if result.Error != nil {
			return nil, result.Error
		}
		all = append(all, result.Item)
	}
	return all, nil
}

func printEvents(events []datadogV2.EventResponse, reason string, dry bool) {
	sep := strings.Repeat("═", 60)
	dryTag := ""
	if dry {
		dryTag = " [DRY RUN]"
	}
	fmt.Printf("\n%s\n", sep)
	fmt.Printf("  EVENTS DISPLAY%s — %d event(s) | Trigger: %s\n", dryTag, len(events), reason)
	fmt.Printf("  Time: %s\n%s\n", time.Now().Format(time.RFC3339), sep)

	for i, e := range events {
		attrs := e.GetAttributes()
		fmt.Printf("\n  [%d/%d] %s\n", i+1, len(events), attrs.GetMessage())
	}

	fmt.Printf("\n%s\n  List reset at %s. Watching for new events...\n%s\n\n", sep, clock(), sep)
}

func clock() string {
	return time.Now().Format("15:04:05")
}

func triggerInvestigation(apiKey, appKey string, events []datadogV2.EventResponse, batchStart time.Time, dry bool) error {
	description := buildDescription(events)

	tagSet := make(map[string]struct{})
	for _, e := range events {
		outer := e.GetAttributes()
		for _, t := range outer.GetTags() {
			tagSet[t] = struct{}{}
		}
	}
	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}

	if dry {
		fmt.Printf("[%s] [DRY RUN] Would trigger Bits AI investigation\n", clock())
		fmt.Printf("  Description: %s\n", description)
		fmt.Printf("  Tags:        %s\n", strings.Join(tags, ", "))
		fmt.Printf("  Start:       %s\n", batchStart.Format(time.RFC3339))
		fmt.Printf("  End:         %s\n", time.Now().Format(time.RFC3339))
		return nil
	}

	body := map[string]any{
		"data": map[string]any{
			"type": "trigger_investigation_request",
			"attributes": map[string]any{
				"trigger": map[string]any{
					"type": "general_investigation",
					"general_investigation": map[string]any{
						"description": description,
						"tags":        tags,
						"start_time":  batchStart.UnixMilli(),
						"end_time":    time.Now().UnixMilli(),
					},
				},
			},
		},
	}
	id, err := postInvestigation(apiKey, appKey, body)
	if err != nil {
		return err
	}
	fmt.Printf("[%s] Bits AI investigation triggered\n", clock())
	if id != "" {
		fmt.Printf("[%s] Investigation URL: https://dddev.datadoghq.com/bits-ai/investigations/%s?section=conclusion&v=trace\n", clock(), id)
	}
	return nil
}

func buildDescription(events []datadogV2.EventResponse) string {
	var parts []string
	for _, e := range events {
		outer := e.GetAttributes()
		msg := outer.GetMessage()
		ts := outer.GetTimestamp().Format(time.RFC3339)
		if msg != "" {
			parts = append(parts, fmt.Sprintf("[%s] %s", ts, msg))
		}
	}
	desc := "Analyse the root cause of the anomaly group"
	if len(parts) > 0 {
		desc += ":\n" + strings.Join(parts, "\n\n")
	}
	return desc
}

func postInvestigation(apiKey, appKey string, body map[string]any) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://app.datadoghq.com/api/v2/bits-ai/investigations", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-APPLICATION-KEY", appKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	fmt.Printf("[%s] [DEBUG] response body: %s\n", clock(), string(respBody))

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.Data.ID != "" {
		return result.Data.ID, nil
	}
	return "", nil
}
