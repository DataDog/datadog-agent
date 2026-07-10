// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"encoding/json"
	"errors"
	"net/http"

	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// RollbackRequest is the JSON body expected by the /agent/ncm/rollback endpoint.
type RollbackRequest struct {
	DeviceID      string `json:"device_id"`
	ConfigVersion string `json:"config_version"`
	Hash          string `json:"hash"`
}

type RollbackResponse struct {
}

// RollbackEndpointHandler returns an http.HandlerFunc for POST /agent/ncm/rollback
func (n *networkDeviceConfigImpl) RollbackEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RollbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputils.SetJSONError(w, err, http.StatusBadRequest)
			return
		}
		result, err := n.RollbackConfig(r.Context(), req.DeviceID, req.ConfigVersion, req.Hash)
		if result == nil && err == nil {
			// this shouldn't be possible.
			httputils.SetJSONError(w, errors.New("no response from RollbackConfig; this should be impossible"), http.StatusInternalServerError)
			return
		}
		if result == nil {
			// we failed before sending anything to the device (bad arguments, or couldn't connect, etc.)
			if errors.Is(err, &ArgumentError{}) {
				httputils.SetJSONError(w, err, http.StatusBadRequest)
			} else {
				httputils.SetJSONError(w, err, http.StatusInternalServerError)
			}
			return
		}
		// result is not nil -> we sent commands to the device, so we need to
		// return information about what we did and what happened.
		if err := result.CopyConfig.AnyError(); err != nil {

		}

		if err != nil {
			// TODO set error code to distinguish between bad requests (e.g.
			// unrecognized device id or hash mismatch) and actual internal
			// errors
			httputils.SetJSONError(w, err, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
