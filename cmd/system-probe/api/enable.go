// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func findModuleFactory(name sysconfigtypes.ModuleName) *module.Factory {
	for _, f := range modules.All() {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func enableModuleHandler(w http.ResponseWriter, r *http.Request, deps module.FactoryDependencies) {
	moduleName := sysconfigtypes.ModuleName(r.PathValue("module_name"))

	target := findModuleFactory(moduleName)
	if target == nil {
		http.Error(w, "invalid module", http.StatusBadRequest)
		return
	}

	if err := module.EnableModule(target, deps); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func disableModuleHandler(w http.ResponseWriter, r *http.Request) {
	moduleName := sysconfigtypes.ModuleName(r.PathValue("module_name"))

	if findModuleFactory(moduleName) == nil {
		http.Error(w, "invalid module", http.StatusBadRequest)
		return
	}

	if err := module.DisableModule(moduleName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
