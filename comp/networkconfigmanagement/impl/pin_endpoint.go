// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"encoding/json"
	"net/http"

	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// PinRequest is the JSON body expected by the /agent/ncm/pin endpoint.
type PinRequest struct {
	DeviceID string `json:"device_id"`
	ConfigID string `json:"config_id"`
	Hash     string `json:"hash"`
}

// PinEndpointHandler returns an http.HandlerFunc for POST /agent/ncm/pin
func (n *networkDeviceConfigImpl) PinEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputils.SetJSONError(w, err, http.StatusBadRequest)
			return
		}
		if err := n.PinConfig(r.Context(), req.DeviceID, req.ConfigID, req.Hash); err != nil {
			httputils.SetJSONError(w, err, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
