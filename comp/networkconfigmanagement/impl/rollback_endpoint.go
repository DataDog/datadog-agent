// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/remote"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// RollbackRequest is the JSON body expected by the /agent/ncm/rollback endpoint.
type RollbackRequest struct {
	DeviceID      string `json:"device_id"`
	ConfigVersion string `json:"config_version"`
	Hash          string `json:"hash"`
}

type RollbackResponse struct {
	CommandResults *remote.PushResult `json:"command_results"`
	ErrorCode      string             `json:"error_code"`
	ErrorMsg       string             `json:"error_msg"`
}

// RollbackEndpointHandler returns an http.HandlerFunc for POST /agent/ncm/rollback
func (n *networkDeviceConfigImpl) RollbackEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RollbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputils.SetJSONError(w, err, http.StatusBadRequest)
			return
		}
		var response RollbackResponse
		result, rberr := n.RollbackConfig(r.Context(), req.DeviceID, req.ConfigVersion, req.Hash)
		if result == nil && rberr == nil {
			// this shouldn't be possible.
			httputils.SetJSONError(w, errors.New("no response from RollbackConfig; this should be impossible"), http.StatusInternalServerError)
			return
		}
		response.CommandResults = result
		if rberr != nil {
			response.ErrorCode = string(rberr.Type())
			response.ErrorMsg = rberr.Error()
		}
		body, err := json.Marshal(response)
		if err != nil {
			httputils.SetJSONError(w, fmt.Errorf("error marshaling response: %w", err), http.StatusInternalServerError)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, string(body))
	}
}
