// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/notableevents"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxNotableEventsAckBodyBytes = 32 << 10
	maxNotableEventsAckIDs       = 100
	maxNotableEventIDLength      = 256
)

type notableEventsCollector interface {
	Start() error
	Close() error
	Pending() []notableevents.Event
	Ack([]string) error
}

// newNotableEventsCollector provides the platform collector constructor and a test seam.
var newNotableEventsCollector = func() (notableEventsCollector, error) {
	return notableevents.NewCollector()
}

// init registers the macOS notable-events module during system-probe startup.
func init() { registerModule(NotableEvents) }

// NotableEvents is the Darwin notable-events module factory.
var NotableEvents = &module.Factory{
	Name: config.NotableEventsModule,
	Fn:   createNotableEventsModule,
}

var _ module.Module = (*notableEventsModule)(nil)

type notableEventsModule struct {
	collector notableEventsCollector
}

type notableEventsAckRequest struct {
	IDs []string `json:"ids"`
}

type notableEventsErrorResponse struct {
	Error string `json:"error"`
}

// createNotableEventsModule constructs and starts the collector backing the module.
func createNotableEventsModule(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
	collector, err := newNotableEventsCollector()
	if err != nil {
		return nil, fmt.Errorf("create notable events collector: %w", err)
	}
	if err := collector.Start(); err != nil {
		if closeErr := collector.Close(); closeErr != nil {
			log.Warnf("Failed to close notable events collector after start failure: %v", closeErr)
		}
		return nil, fmt.Errorf("start notable events collector: %w", err)
	}
	return &notableEventsModule{collector: collector}, nil
}

// Register exposes the pending-event and acknowledgement endpoints.
func (m *notableEventsModule) Register(router *module.Router) error {
	router.HandleFunc("GET /check", m.handleCheck)
	router.HandleFunc("POST /ack", m.handleAck)
	return nil
}

// handleCheck returns a snapshot of events awaiting forwarding-pipeline acceptance.
func (m *notableEventsModule) handleCheck(w http.ResponseWriter, req *http.Request) {
	writeNotableEventsJSON(req, w, http.StatusOK, m.collector.Pending())
}

// handleAck validates and persists acknowledgement of accepted events.
func (m *notableEventsModule) handleAck(w http.ResponseWriter, req *http.Request) {
	body, status, err := decodeNotableEventsAckRequest(w, req)
	if err != nil {
		writeNotableEventsJSON(req, w, status, notableEventsErrorResponse{Error: err.Error()})
		return
	}
	if err := m.collector.Ack(body.IDs); err != nil {
		log.Errorf("Failed to acknowledge notable events: %v", err)
		writeNotableEventsJSON(req, w, http.StatusInternalServerError, notableEventsErrorResponse{Error: "failed to acknowledge notable events"})
		return
	}
	writeNotableEventsJSON(req, w, http.StatusOK, struct{}{})
}

// decodeNotableEventsAckRequest decodes a bounded acknowledgement request and validates its IDs.
func decodeNotableEventsAckRequest(w http.ResponseWriter, req *http.Request) (notableEventsAckRequest, int, error) {
	var body notableEventsAckRequest
	req.Body = http.MaxBytesReader(w, req.Body, maxNotableEventsAckBodyBytes)
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&body); err != nil {
		status, decodeErr := notableEventsDecodeError(err)
		return body, status, decodeErr
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		status, decodeErr := notableEventsDecodeError(err)
		return body, status, decodeErr
	}
	if body.IDs == nil {
		return body, http.StatusBadRequest, errors.New("ids is required")
	}
	if len(body.IDs) > maxNotableEventsAckIDs {
		return body, http.StatusBadRequest, fmt.Errorf("ids must contain at most %d entries", maxNotableEventsAckIDs)
	}
	for _, id := range body.IDs {
		if strings.TrimSpace(id) == "" {
			return body, http.StatusBadRequest, errors.New("ids must not contain empty values")
		}
		if len(id) > maxNotableEventIDLength {
			return body, http.StatusBadRequest, fmt.Errorf("ids must not exceed %d bytes", maxNotableEventIDLength)
		}
	}
	return body, http.StatusOK, nil
}

// notableEventsDecodeError maps JSON decoding failures to safe HTTP responses.
func notableEventsDecodeError(err error) (int, error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge, fmt.Errorf("request body exceeds %d bytes", maxNotableEventsAckBodyBytes)
	}
	return http.StatusBadRequest, fmt.Errorf("invalid JSON request: %w", err)
}

// writeNotableEventsJSON writes a compact JSON response with the requested status.
func writeNotableEventsJSON(req *http.Request, w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	utils.WriteAsJSON(req, w, body, utils.CompactOutput)
}

// GetStats returns the module's currently empty runtime statistics.
func (m *notableEventsModule) GetStats() map[string]interface{} {
	return map[string]interface{}{}
}

// Close releases the collector and its filesystem monitoring resources.
func (m *notableEventsModule) Close() {
	if err := m.collector.Close(); err != nil {
		log.Warnf("Failed to close notable events collector: %v", err)
	}
}
