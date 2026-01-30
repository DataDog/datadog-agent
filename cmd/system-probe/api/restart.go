// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func restartModuleHandler(w http.ResponseWriter, r *http.Request, modules []types.SystemProbeModuleComponent) {
	vars := mux.Vars(r)
	moduleName := sysconfigtypes.ModuleName(vars["module-name"])

	if moduleName == config.EventMonitorModule {
		w.WriteHeader(http.StatusOK)
		return
	}

	var target types.SystemProbeModuleComponent
	for _, mod := range modules {
		if mod.Name() == moduleName {
			target = mod
		}
	}

	if target == nil || target.Name() != moduleName {
		http.Error(w, "invalid module", http.StatusBadRequest)
		return
	}

	err := module.RestartModule(target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
