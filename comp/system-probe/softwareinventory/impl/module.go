// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package impl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type softwareInventoryModule struct {
}

func (sim *softwareInventoryModule) Register(httpMux types.SystemProbeRouter) error {
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
		utils.WriteAsJSON(w, inventory, utils.CompactOutput)
	}))

	return nil
}

func (sim *softwareInventoryModule) GetStats() map[string]interface{} {
	return map[string]interface{}{}
}

func (sim *softwareInventoryModule) Close() {

}
