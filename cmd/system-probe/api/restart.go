// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func restartModuleHandler(w http.ResponseWriter, r *http.Request, wmeta optional.Option[workloadmeta.Component]) {
	vars := mux.Vars(r)
	moduleName := sysconfigtypes.ModuleName(vars["module-name"])

	if moduleName == config.EventMonitorModule {
		w.WriteHeader(http.StatusOK)
		return
	}

	var target module.Factory
	for _, f := range modules.All {
		if f.Name == moduleName {
			target = f
		}
	}

	if target.Name != moduleName {
		http.Error(w, "invalid module", http.StatusBadRequest)
		return
	}

	err := module.RestartModule(target, wmeta)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
