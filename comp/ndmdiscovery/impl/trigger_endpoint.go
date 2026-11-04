// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
	"errors"
	"net/http"

	discoverytypes "github.com/DataDog/datadog-agent/pkg/networkdevices/discovery"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// TriggerDiscoveryEndpointHandler sweeps one range now, on request. It answers
// as soon as the sweep is queued, since a range can take hours to scan.
func (d *ndmDiscovery) TriggerDiscoveryEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req discoverytypes.TriggerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputils.SetJSONError(w, err, http.StatusBadRequest)
			return
		}

		if err := d.Trigger(req.AutodiscoveryID); err != nil {
			d.log.Infof("ndmdiscovery: refused a triggered sweep: %v", err)
			httputils.SetJSONError(w, err, triggerStatus(err))
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// triggerStatus maps a refusal to the status that describes it, so the caller
// can tell a retryable conflict from a configuration problem.
func triggerStatus(err error) int {
	switch {
	case errors.Is(err, discoverytypes.ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, errUnknownRange):
		return http.StatusNotFound
	case errors.Is(err, errAlreadySweeping):
		return http.StatusConflict
	case errors.Is(err, errDisabled):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
