// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package modules

import (
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	vdimodel "github.com/DataDog/datadog-agent/pkg/vdi/model"
	"github.com/DataDog/datadog-agent/pkg/vdi/provider/avd"
	windowsessions "github.com/DataDog/datadog-agent/pkg/vdi/session/windows"
)

func init() { registerModule(VDI) }

// VDI is the Windows virtual desktop inventory module factory.
var VDI = &module.Factory{
	Name: sysconfigtypes.VDIModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return newVDIModule(windowsessions.EnumerateSessions), nil
	},
}

type vdiModule struct {
	windowsFn func() ([]vdimodel.WindowsSession, error)
}

func newVDIModule(windowsFn func() ([]vdimodel.WindowsSession, error)) *vdiModule {
	return &vdiModule{windowsFn: windowsFn}
}

func (m *vdiModule) Register(router *module.Router) error {
	router.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		response := m.inventory()
		utils.WriteAsJSON(req, w, response, utils.CompactOutput)
	}))
	return nil
}

func (m *vdiModule) inventory() vdimodel.InventoryResponse {
	response := vdimodel.InventoryResponse{
		CollectedAt: time.Now().UTC(),
		Providers:   make(map[string]vdimodel.ProviderInventory, 1),
	}
	windowsInventory, err := m.windowsFn()
	if err != nil {
		response.Windows.SourceStatus = vdimodel.SourceStatus{Status: vdimodel.StatusError, Error: err.Error()}
	} else {
		response.Windows.SourceStatus = vdimodel.SourceStatus{Status: vdimodel.StatusOK}
		response.Windows.Sessions = windowsInventory
	}
	response.Providers[vdimodel.ProviderAzureVirtualDesktop] = avd.FromWindowsInventory(response.Windows)

	return response
}

func (m *vdiModule) GetStats() map[string]interface{} { return map[string]interface{}{} }

func (m *vdiModule) Close() {}
