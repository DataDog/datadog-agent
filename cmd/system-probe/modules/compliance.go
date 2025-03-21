// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build linux

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/compliance/dbconfig"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"go.uber.org/atomic"
)

// ComplianceModule is a system-probe module that exposes an HTTP api to
// perform compliance checks that require more privileges than security-agent
// can offer.
//
// For instance, being able to run cross-container checks at runtime by directly
// accessing the /proc/<pid>/root mount point.
var ComplianceModule = module.Factory{
	Name:             config.ComplianceModule,
	ConfigNamespaces: []string{},
	Fn: func(cfg *sysconfigtypes.Config, _ optional.Option[workloadmeta.Component]) (module.Module, error) {
		return &complianceModule{}, nil
	},
	NeedsEBPF: func() bool {
		return false
	},
}

type complianceModule struct {
	performedChecks atomic.Uint64
}

// Close is a noop (implements module.Module)
func (*complianceModule) Close() {
}

// GetStats returns statistics related to the compliance module (implements module.Module)
func (m *complianceModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"performed_checks": m.performedChecks.Load(),
	}
}

// Register implements module.Module.
func (m *complianceModule) Register(router *module.Router) error {
	router.HandleFunc("/dbconfig", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests, m.handleScanDBConfig))
	return nil
}

func (m *complianceModule) handleError(writer http.ResponseWriter, request *http.Request, status int, err error) {
	_ = log.Errorf("module compliance: failed to properly handle %s request: %s", request.URL.Path, err)
	writer.Header().Set("Content-Type", "text/plain")
	writer.WriteHeader(status)
	writer.Write([]byte(err.Error()))
}

func (m *complianceModule) handleScanDBConfig(writer http.ResponseWriter, request *http.Request) {
	m.performedChecks.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	qs := request.URL.Query()
	pid, err := strconv.ParseInt(qs.Get("pid"), 10, 32)
	if err != nil {
		m.handleError(writer, request, http.StatusBadRequest, fmt.Errorf("pid query parameter is not an integer: %w", err))
		return
	}

	resource, ok := dbconfig.LoadDBResourceFromPID(ctx, int32(pid))
	if !ok {
		m.handleError(writer, request, http.StatusNotFound, fmt.Errorf("resource not found for pid=%d", pid))
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	e := json.NewEncoder(writer)
	if err := e.Encode(resource); err != nil {
		_ = log.Errorf("module compliance: failed to properly handle %s request: could not send response %s", request.URL.Path, err)
	}
}
