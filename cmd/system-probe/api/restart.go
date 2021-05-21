package api

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/gorilla/mux"
)

func restartModuleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	moduleName := config.ModuleName(vars["module-name"])

	var target module.Factory
	for _, f := range modules.All {
		if f.Name == moduleName {
			target = f
		}
	}

	if target.Name != moduleName {
		http.Error(w, fmt.Sprintf("invalid module: %s", moduleName), http.StatusBadRequest)
		return
	}

	err := module.RestartModule(target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
