// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/test/fakeintake/server/serverstore"
)

func (fi *Server) handleGetPayloadPage(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	q := req.URL.Query()
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || q.Get("endpoint") == "" || limit < 1 || limit > serverstore.MaxPageSize {
		http.Error(w, "endpoint and limit (1..128) required", http.StatusBadRequest)
		return
	}
	page, err := fi.store.GetRawPayloadPage(q.Get("endpoint"), q.Get("cursor"), limit)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, serverstore.ErrExpiredCursor) {
			status = http.StatusGone
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// The store snapshot has independent bodies and metadata; network I/O holds no store lock.
	if err = json.NewEncoder(w).Encode(page); err != nil {
		return
	} // The reader observes a failed or incomplete response.
}
