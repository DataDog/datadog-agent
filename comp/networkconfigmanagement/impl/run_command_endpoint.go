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

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// RunCommandRequest is the JSON body expected by the /agent/ncm/run-command endpoint.
type RunCommandRequest struct {
	DeviceID string `json:"device_id"`
	Command  string `json:"command"`
}

// RunCommandEndpointHandler returns an http.HandlerFunc for POST /agent/ncm/run-command
func (n *networkDeviceConfigImpl) RunCommandEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RunCommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputils.SetJSONError(w, err, http.StatusBadRequest)
			return
		}
		var response types.RunCommandResponse
		result, rcerr := n.RunCommand(r.Context(), req.DeviceID, req.Command)
		if result == nil && rcerr == nil {
			// this shouldn't be possible.
			httputils.SetJSONError(w, errors.New("no response from RunCommand; this should be impossible"), http.StatusInternalServerError)
			return
		}
		response.CommandResult = result
		if rcerr != nil {
			response.ErrorCode = string(rcerr.Type())
			response.ErrorMsg = rcerr.Error()
		}
		body, err := json.Marshal(response)
		if err != nil {
			httputils.SetJSONError(w, fmt.Errorf("error marshaling response: %w", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, string(body))
	}
}
