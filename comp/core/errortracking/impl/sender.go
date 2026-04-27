// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package errortrackingimpl implements the COAT error tracking sender
// component. It POSTs batches of slog.Records to the apmtelemetry intake.
// Batching, retries-from-the-pipeline, and processor execution are NOT
// performed here — Worker 1's in-package Pipeline at
// pkg/util/log/errortracking owns those concerns and calls Send once per
// batch.
package errortrackingimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const (
	apiVersion       = "v2"
	intakeHostPrefix = "https://instrumentation-telemetry-intake."
	intakePath       = "/api/v2/apmtelemetry"

	// requestType discriminates the apmtelemetry envelope at the receiver.
	// "logs" routes through dd-go's existing logs processor for v1 staging
	// validation (see ARCH_NOTES_coat_intake.md §3); long-term we move to
	// "agent-errortracking" once the receiver-side branch lands.
	requestType = "logs"

	// serviceName is the Service field on the inner ErrorTracking payload.
	// The receiver tags emitted records with this; keeping it stable lets
	// us write Datadog UI queries like service:datadog-agent.
	serviceName = "datadog-agent"

	httpClientTimeout = 10 * time.Second
)

// httpDoer is the minimal HTTP client surface senderImpl uses; tests inject
// httptest-backed clients through this.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// senderImpl is the batch-level errortracking.Sender implementation. It owns
// JSON encoding, headers, transport, and status-code semantics; nothing
// stateful about the queue lives here.
type senderImpl struct {
	logComp log.Component
	client  httpDoer

	url    string
	apiKey string

	agentVersion string
	hostname     string
	requestType  string

	stopped atomic.Bool
}

// newSenderImpl constructs a senderImpl. Tests use this directly; Fx wiring
// uses NewComponent in component.go which calls through to here.
func newSenderImpl(
	logComp log.Component,
	client httpDoer,
	url, apiKey, agentVersion, hostname string,
) *senderImpl {
	return &senderImpl{
		logComp:      logComp,
		client:       client,
		url:          url,
		apiKey:       apiKey,
		agentVersion: agentVersion,
		hostname:     hostname,
		requestType:  requestType,
	}
}

// Send implements errortracking.Sender. It encodes the batch into the
// apmtelemetry envelope and POSTs once; the pipeline retries the call once
// on a non-nil error before dropping the batch.
//
// Status code semantics (per ARCH_NOTES_coat_intake.md §1):
//   - 2xx: nil
//   - 4xx: terminal — log and return nil so the pipeline does not waste a retry
//   - 5xx, network/timeout: non-nil error so the pipeline retries once
func (s *senderImpl) Send(ctx context.Context, batch []slog.Record) error {
	if s.stopped.Load() {
		return nil
	}
	if len(batch) == 0 {
		return nil
	}

	payload := s.buildPayload(batch)
	body, err := json.Marshal(payload)
	if err != nil {
		s.logComp.Errorf("errortracking: failed to marshal payload: %v", err)
		return nil
	}

	body, err = scrubber.ScrubJSON(body)
	if err != nil {
		s.logComp.Errorf("errortracking: failed to scrub payload: %v", err)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		s.logComp.Errorf("errortracking: failed to build request: %v", err)
		return nil
	}
	s.addHeaders(req, len(body))

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("errortracking: transport error posting to %s: %w", s.url, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode >= 500:
		return fmt.Errorf("errortracking: intake returned %s", resp.Status)
	default:
		s.logComp.Debugf("errortracking: intake rejected payload: status=%d url=%s", resp.StatusCode, s.url)
		return nil
	}
}

// buildPayload converts a batch of slog.Records into the wire envelope.
func (s *senderImpl) buildPayload(batch []slog.Record) Payload {
	records := make([]ErrorRecord, 0, len(batch))
	for _, r := range batch {
		records = append(records, recordFromSlog(r))
	}
	return Payload{
		APIVersion:  apiVersion,
		RequestType: s.requestType,
		EventTime:   time.Now().UnixMilli(),
		Host:        HostPayload{Hostname: s.hostname},
		Payload: ErrorTracking{
			AgentVersion: s.agentVersion,
			Hostname:     s.hostname,
			Service:      serviceName,
			Records:      records,
		},
	}
}

// recordFromSlog flattens a slog.Record into an ErrorRecord. Attrs are
// stringified via slog.Value.String so structured values (durations, errors,
// numbers) survive the round-trip.
func recordFromSlog(r slog.Record) ErrorRecord {
	out := ErrorRecord{
		Time:    r.Time.UTC().Format(time.RFC3339Nano),
		Level:   r.Level.String(),
		Message: r.Message,
	}
	if r.NumAttrs() > 0 {
		out.Attrs = make(map[string]string, r.NumAttrs())
		r.Attrs(func(a slog.Attr) bool {
			out.Attrs[a.Key] = a.Value.String()
			return true
		})
	}
	return out
}

func (s *senderImpl) addHeaders(req *http.Request, bodyLen int) {
	req.Header.Set("DD-Api-Key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(bodyLen))
	req.Header.Set("DD-Telemetry-api-version", apiVersion)
	req.Header.Set("DD-Telemetry-request-type", s.requestType)
	req.Header.Set("DD-Telemetry-Product", "agent")
	req.Header.Set("DD-Telemetry-Product-Version", s.agentVersion)
	req.Header.Set("User-Agent", "Datadog Agent/"+s.agentVersion)
}

// markStopped is invoked from the Fx OnStop hook so any in-flight Send call
// returns early instead of opening a new connection during shutdown.
func (s *senderImpl) markStopped() {
	s.stopped.Store(true)
}
