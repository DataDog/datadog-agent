// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkconfigmanagementimpl

import (
	"encoding/json"
	"net/http"

	types "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/types"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// GetConfigResponse is the JSON response returned by the /agent/ncm/config endpoint.
type GetConfigResponse struct {
	ConfigUUID string           `json:"config_uuid"`
	DeviceID   string           `json:"device_id"`
	ConfigType types.ConfigType `json:"config_type"`
	CapturedAt int64            `json:"captured_at"`
	RawConfig  string           `json:"raw_config"`
}

// GetConfigEndpointHandler returns an http.HandlerFunc for GET /agent/ncm/config?uuid=<uuid>.
func (n *networkDeviceConfigImpl) GetConfigEndpointHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if n.store == nil {
			http.Error(w, `{"error": "ncm rollbacks not available for agent"}`, http.StatusBadRequest)
			return
		}
		uuid := r.URL.Query().Get("uuid")
		if uuid == "" {
			http.Error(w, `{"error": "missing uuid query parameter"}`, http.StatusBadRequest)
			return
		}

		rawConfig, metadata, err := n.store.GetConfig(uuid)
		if err != nil {
			httputils.SetJSONError(w, err, http.StatusNotFound)
			return
		}

		resp := GetConfigResponse{
			ConfigUUID: metadata.ConfigUUID,
			DeviceID:   metadata.DeviceID,
			ConfigType: metadata.ConfigType,
			CapturedAt: metadata.CapturedAt,
			RawConfig:  rawConfig,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			httputils.SetJSONError(w, err, http.StatusInternalServerError)
		}
	}
}
