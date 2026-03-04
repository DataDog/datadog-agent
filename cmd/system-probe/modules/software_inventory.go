// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin || windows

package modules

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
		wireEntries := make([]software.SoftwareInventoryWireEntry, len(inventory))
		for i, entry := range inventory {
			wireEntries[i] = software.EntryToWire(entry)
		}
		utils.WriteAsJSON(w, wireEntries, utils.CompactOutput)
	}))

	return nil
}

func (sim *softwareInventoryModule) GetStats() map[string]interface{} {
	return map[string]interface{}{}
}

func (sim *softwareInventoryModule) Close() {}
