// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed by Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package modules

import (
	"context"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	vdimodel "github.com/DataDog/datadog-agent/pkg/vdi/model"
	"github.com/DataDog/datadog-agent/pkg/vdi/provider/dcv"
	windowsessions "github.com/DataDog/datadog-agent/pkg/vdi/session/windows"
)

func init() { registerModule(VDI) }

// VDI is the Windows virtual desktop inventory module factory.
var VDI = &module.Factory{
	Name: config.VDIModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return newVDIModule(dcv.NewCollector(dcv.CommandRunner{}), windowsessions.EnumerateSessions), nil
	},
}

type dcvCollector interface {
	Collect(context.Context) vdimodel.ProviderInventory
}

type vdiModule struct {
	dcv       dcvCollector
	windowsFn func() ([]vdimodel.WindowsSession, error)
}

func newVDIModule(dcvCollector dcvCollector, windowsFn func() ([]vdimodel.WindowsSession, error)) *vdiModule {
	return &vdiModule{dcv: dcvCollector, windowsFn: windowsFn}
}

func (m *vdiModule) Register(router *module.Router) error {
	router.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		response := m.inventory(req.Context())
		utils.WriteAsJSON(req, w, response, utils.CompactOutput)
	}))
	return nil
}

func (m *vdiModule) inventory(ctx context.Context) vdimodel.InventoryResponse {
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
	response.Providers[vdimodel.ProviderAWSWorkSpaces] = m.dcv.Collect(ctx)
	return response
}

func (m *vdiModule) GetStats() map[string]interface{} { return map[string]interface{}{} }

func (m *vdiModule) Close() {}
