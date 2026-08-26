// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
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
)

func init() { registerModule(VDI) }

// VDI is the Windows virtual desktop inventory module factory.
var VDI = &module.Factory{
	Name: config.VDIModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		return newVDIModule(dcv.NewCollector(dcv.CommandRunner{})), nil
	},
}

type vdiCollector interface {
	Provider() string
	Collect(context.Context) vdimodel.ProviderInventory
}

type vdiModule struct {
	collectors []vdiCollector
}

func newVDIModule(collectors ...vdiCollector) *vdiModule {
	return &vdiModule{collectors: collectors}
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
		Providers:   make(map[string]vdimodel.ProviderInventory, len(m.collectors)),
	}
	for _, collector := range m.collectors {
		response.Providers[collector.Provider()] = collector.Collect(ctx)
	}
	return response
}

func (m *vdiModule) GetStats() map[string]interface{} { return map[string]interface{}{} }

func (m *vdiModule) Close() {}
