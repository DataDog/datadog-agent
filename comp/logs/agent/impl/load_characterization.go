// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/characterization"
)

const loadCharacterizationRoute = "/logs/load-characterization"

type startCharacterizationRequest struct {
	DurationSeconds float64 `json:"duration_seconds"`
}

func loadCharacterizationHandler(manager *characterization.Manager, allowed bool) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if !allowed {
			writeCharacterizationError(writer, http.StatusNotFound, "logs characterization is not allowed")
			return
		}
		switch request.Method {
		case http.MethodPost:
			startCharacterization(writer, request, manager)
		case http.MethodGet:
			statusCharacterization(writer, manager)
		case http.MethodDelete:
			stopCharacterization(writer, request, manager)
		default:
			writer.Header().Set("Allow", "POST, GET, DELETE")
			writeCharacterizationError(writer, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func startCharacterization(writer http.ResponseWriter, request *http.Request, manager *characterization.Manager) {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4096))
	decoder.DisallowUnknownFields()
	var body startCharacterizationRequest
	if err := decoder.Decode(&body); err != nil {
		writeCharacterizationError(writer, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeCharacterizationError(writer, http.StatusBadRequest, "invalid request body")
		return
	}
	snapshot, err := manager.Start(time.Duration(body.DurationSeconds * float64(time.Second)))
	if err != nil {
		if errors.Is(err, characterization.ErrSessionActive) {
			writeCharacterizationError(writer, http.StatusConflict, err.Error())
			return
		}
		writeCharacterizationError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeCharacterizationJSON(writer, http.StatusCreated, snapshot)
}

func statusCharacterization(writer http.ResponseWriter, manager *characterization.Manager) {
	snapshot, err := manager.Status()
	if err != nil {
		writeCharacterizationError(writer, http.StatusNotFound, err.Error())
		return
	}
	writeCharacterizationJSON(writer, http.StatusOK, snapshot)
}

func stopCharacterization(writer http.ResponseWriter, request *http.Request, manager *characterization.Manager) {
	snapshot, err := manager.Stop(request.URL.Query().Get("session_id"))
	if err != nil {
		writeCharacterizationError(writer, http.StatusNotFound, err.Error())
		return
	}
	writeCharacterizationJSON(writer, http.StatusOK, snapshot)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeCharacterizationJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeCharacterizationError(writer http.ResponseWriter, status int, message string) {
	writeCharacterizationJSON(writer, status, map[string]string{"error": message})
}
