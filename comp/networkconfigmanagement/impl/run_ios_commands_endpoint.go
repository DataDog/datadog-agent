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

	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/ioscmd"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// RunIOSCommandsRequest is the JSON body expected by the /agent/ncm/run-ios-commands endpoint.
type RunIOSCommandsRequest struct {
	DeviceID string `json:"device_id"`
	// Command is a JSON-encoded ioscmd.CommandBlock; it is parsed and validated
	// by the endpoint.
	Command json.RawMessage `json:"command"`
}

func writeRunIOSCommandsResponse(w http.ResponseWriter, response types.RunIOSCommandsResponse) {
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

// RunIOSCommandsEndpointHandler returns an http.HandlerFunc for POST /agent/ncm/run-ios-commands
func (n *networkDeviceConfigImpl) RunIOSCommandsEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RunIOSCommandsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputils.SetJSONError(w, err, http.StatusBadRequest)
			return
		}
		commands, err := ioscmd.Parse(req.Command)
		if err != nil {
			writeRunIOSCommandsResponse(w, types.RunIOSCommandsResponse{
				ErrorCode: string(types.ErrInvalidCommand),
				ErrorMsg:  fmt.Sprintf("invalid command: %v", err),
			})
			return
		}
		var response types.RunIOSCommandsResponse
		results, rcerr := n.RunIOSCommands(r.Context(), req.DeviceID, commands)
		if results == nil && rcerr == nil {
			// this shouldn't be possible.
			httputils.SetJSONError(w, errors.New("no response from RunIOSCommands; this should be impossible"), http.StatusInternalServerError)
			return
		}
		response.CommandResults = results
		if rcerr != nil {
			response.ErrorCode = string(rcerr.Type())
			response.ErrorMsg = rcerr.Error()
		}
		writeRunIOSCommandsResponse(w, response)
	}
}
