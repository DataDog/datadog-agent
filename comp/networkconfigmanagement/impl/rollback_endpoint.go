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

// RollbackRequest is the JSON body expected by the /agent/ncm/rollback endpoint.
type RollbackRequest struct {
	DeviceID      string `json:"device_id"`
	ConfigVersion string `json:"config_version"`
	Hash          string `json:"hash"`
}

// RollbackEndpointHandler returns an http.HandlerFunc for POST /agent/ncm/rollback
func (n *networkDeviceConfigImpl) RollbackEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RollbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputils.SetJSONError(w, err, http.StatusBadRequest)
			return
		}
		if err := n.RollbackConfig(r.Context(), req.DeviceID, req.ConfigVersion, req.Hash); err != nil {
			// TODO set error code to distinguish between bad requests (e.g.
			// unrecognized device id or hash mismatch) and actual internal
			// errors
			httputils.SetJSONError(w, err, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
