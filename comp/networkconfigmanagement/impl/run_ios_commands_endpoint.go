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

	ncmconfig "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/config"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/ioscmd"
	"github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// RunIOSCommandsRequest is the JSON body expected by the /agent/ncm/run-ios-commands endpoint.
type RunIOSCommandsRequest struct {
	DeviceID string `json:"device_id"`
	// Command is a JSON-encoded ioscmd.CommandBlock; it is parsed and validated
	// by the endpoint.
	Command       json.RawMessage `json:"command"`
	CredentialSet string          `json:"credential_set"`
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
		credentialSet, err := ncmconfig.ParseCredentialSet(req.CredentialSet)
		if err != nil {
			writeRunIOSCommandsResponse(w, types.RunIOSCommandsResponse{
				ErrorCode: string(types.ErrCannotConnect),
				ErrorMsg:  err.Error(),
			})
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
		result, rcerr := n.RunIOSCommands(r.Context(), req.DeviceID, commands, credentialSet)
		if result == nil && rcerr == nil {
			// this shouldn't be possible.
			httputils.SetJSONError(w, errors.New("no response from RunIOSCommands; this should be impossible"), http.StatusInternalServerError)
			return
		}
		response.CommandResult = result
		if rcerr != nil {
			response.ErrorCode = string(rcerr.Type())
			response.ErrorMsg = rcerr.Error()
		}
		writeRunIOSCommandsResponse(w, response)
	}
}
