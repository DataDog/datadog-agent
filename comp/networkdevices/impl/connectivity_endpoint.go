// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkdevicesimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

func (c *networkDevicesImpl) ConnectivityCheckEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req connectivity.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputils.SetJSONError(w, err, http.StatusBadRequest)
			return
		}

		res, err := c.CheckConnectivity(r.Context(), req)
		if err != nil {
			c.logger.Errorf("networkdevices: connectivity check failed: %v", err)
			status := http.StatusInternalServerError
			if errors.Is(err, connectivity.ErrInvalidRequest) {
				status = http.StatusBadRequest
			}
			httputils.SetJSONError(w, err, status)
			return
		}

		body, err := json.Marshal(res)
		if err != nil {
			httputils.SetJSONError(w, fmt.Errorf("error marshaling response: %w", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(body); err != nil {
			c.logger.Errorf("networkdevices: failed to write response: %v", err)
		}
	}
}
