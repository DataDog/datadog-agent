// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/api/coverage"
)

//nolint:revive // TODO(PROC) Fix revive linter
type APIServerDeps struct {
	fx.In

	Config       config.Component
	Log          log.Component
	WorkloadMeta workloadmeta.Component
	Status       status.Component
	Settings     settings.Component
	Tagger       tagger.Component
	Secrets      secrets.Component
}

func injectDeps(deps APIServerDeps, handler func(APIServerDeps, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(writer http.ResponseWriter, req *http.Request) {
		handler(deps, writer, req)
	}
}

//nolint:revive // TODO(PROC) Fix revive linter
func SetupAPIServerHandlers(deps APIServerDeps, r *http.ServeMux) {
	// Routes are registered through observability.WrapWithRouteTemplate so the
	// api_server request telemetry reports the route template in the path tag
	// rather than "unknown".
	observability.WrapWithRouteTemplate(r, "GET", "/config", http.HandlerFunc(deps.Settings.GetFullConfig("process_config")))
	observability.WrapWithRouteTemplate(r, "GET", "/config/without-defaults", http.HandlerFunc(deps.Settings.GetFullConfigWithoutDefaults("process_config")))
	observability.WrapWithRouteTemplate(r, "GET", "/config/all", http.HandlerFunc(deps.Settings.GetFullConfig(""))) // Get all fields from process-agent Config object
	observability.WrapWithRouteTemplate(r, "GET", "/config/list-runtime", http.HandlerFunc(deps.Settings.ListConfigurable))
	observability.WrapWithRouteTemplate(r, "GET", "/config/{setting}", http.HandlerFunc(deps.Settings.GetValue))
	observability.WrapWithRouteTemplate(r, "POST", "/config/{setting}", http.HandlerFunc(deps.Settings.SetValue))

	observability.WrapWithRouteTemplate(r, "GET", "/agent/status", injectDeps(deps, statusHandler))
	observability.WrapWithRouteTemplate(r, "GET", "/agent/tagger-list", injectDeps(deps, getTaggerList))
	observability.WrapWithRouteTemplate(r, "GET", "/agent/workload-list/short", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		workloadList(w, false, deps.WorkloadMeta)
	}))
	observability.WrapWithRouteTemplate(r, "GET", "/agent/workload-list/verbose", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		workloadList(w, true, deps.WorkloadMeta)
	}))
	observability.WrapWithRouteTemplate(r, "GET", "/check/{check}", http.HandlerFunc(checkHandler))
	observability.WrapWithRouteTemplate(r, "GET", "/secret/refresh", injectDeps(deps, secretRefreshHandler))
	// Special handler to compute running agent Code coverage
	coverage.SetupCoverageHandler(r)
}
