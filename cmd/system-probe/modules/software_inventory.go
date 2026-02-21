// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin || windows

package modules

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// systemProbeEntry is a wrapper around software.Entry that includes InstallPath
// in JSON for system-probe internal communication. This ensures InstallPath
// is preserved when data is serialized/deserialized between system-probe and agent.
type systemProbeEntry struct {
	software.Entry
	// InstallPath is included in JSON for system-probe communication
	// This is needed for proper deduplication (GetID uses InstallPath)
	InstallPathInternal string `json:"install_path,omitempty"`
}

// MarshalJSON customizes JSON marshaling to include InstallPath
func (e *systemProbeEntry) MarshalJSON() ([]byte, error) {
	// Create a type alias to avoid infinite recursion
	type Alias software.Entry
	aux := &struct {
		*Alias
		InstallPathInternal string `json:"install_path,omitempty"`
	}{
		Alias:               (*Alias)(&e.Entry),
		InstallPathInternal: e.Entry.InstallPath,
	}
	return json.Marshal(aux)
}

// UnmarshalJSON customizes JSON unmarshaling to restore InstallPath
func (e *systemProbeEntry) UnmarshalJSON(data []byte) error {
	type Alias software.Entry
	aux := &struct {
		*Alias
		InstallPathInternal string `json:"install_path,omitempty"`
	}{
		Alias: (*Alias)(&e.Entry),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	// Restore InstallPath from the JSON field
	e.Entry.InstallPath = aux.InstallPathInternal
	return nil
}

// toSystemProbeEntries converts []*software.Entry to []*systemProbeEntry
func toSystemProbeEntries(entries []*software.Entry) []*systemProbeEntry {
	result := make([]*systemProbeEntry, len(entries))
	for i, entry := range entries {
		result[i] = &systemProbeEntry{Entry: *entry}
	}
	return result
}

func init() { registerModule(SoftwareInventory) }

// SoftwareInventory Factory
var SoftwareInventory = &module.Factory{
	Name:             config.SoftwareInventoryModule,
	ConfigNamespaces: []string{"software_inventory"},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return &softwareInventoryModule{}, nil
	},
}

var _ module.Module = &softwareInventoryModule{}

type softwareInventoryModule struct{}

func (sim *softwareInventoryModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, _ *http.Request) {
		log.Infof("Got check request in software inventory")
		inventory, warnings, err := software.GetSoftwareInventory()
		if err != nil {
			log.Errorf("Error getting software inventory: %v", err)
			w.WriteHeader(500)
			return
		}
		for _, warning := range warnings {
			_ = log.Warnf("warning: %s", warning)
		}
		// Convert to systemProbeEntry to include InstallPath in JSON
		// This ensures InstallPath is preserved for proper deduplication
		sysProbeInventory := toSystemProbeEntries(inventory)
		utils.WriteAsJSON(w, sysProbeInventory, utils.CompactOutput)
	}))

	return nil
}

func (sim *softwareInventoryModule) GetStats() map[string]interface{} {
	return map[string]interface{}{}
}

func (sim *softwareInventoryModule) Close() {}
