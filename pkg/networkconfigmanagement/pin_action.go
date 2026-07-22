// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkconfigmanagement provides types and utilities shared across NCM sub-packages.
package networkconfigmanagement

// PinConfigHandler is invoked by the PAR action runner via POST /agent/ncm/pin,
// which is wired up in comp/networkconfigmanagement/impl/pin_endpoint.go.
// The Component.PinConfig method (impl/pin_config.go) owns hash verification
// and the store.SetPinned call; this function is the package-level entry point
// for callers that hold a store directly (e.g. tests).

import (
	"fmt"

	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
)

// PinConfigHandler handles the "pinConfig" PAR action.
// It verifies the stored config hash matches the provided hash, then marks the
// config as pinned so it is excluded from future eviction runs.
func PinConfigHandler(store ncmstore.ConfigStore, deviceID string, configID string, hash string) error {
	_, metadata, err := store.GetConfig(configID)
	if err != nil {
		return fmt.Errorf("config not found in local store (may have been evicted)")
	}

	_ = deviceID // validated by the caller via the device registry

	if metadata.RawHash != hash {
		return fmt.Errorf("hash mismatch")
	}

	return store.SetPinned(configID, true)
}
