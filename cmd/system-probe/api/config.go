// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
)

// setupConfigHandlers adds the specific handlers for /config endpoints
func setupConfigHandlers(r *http.ServeMux, settings settings.Component) {
	r.HandleFunc("GET /config", settings.GetFullConfig(""))
	r.HandleFunc("GET /config/without-defaults", settings.GetFullConfigWithoutDefaults(""))
	r.HandleFunc("GET /config/by-source", settings.GetFullConfigBySource())
	r.HandleFunc("GET /config/list-runtime", settings.ListConfigurable)
	r.HandleFunc("GET /config/{setting}", settings.GetValue)
	r.HandleFunc("POST /config/{setting}", settings.SetValue)
}
