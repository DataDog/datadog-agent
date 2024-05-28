// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"net/http"

	"encoding/json"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var mimeTypeMap = map[string]string{
	"text": "text/plain",
	"json": "application/json",
}

func (s *statusImplementation) apiEndpoints() []api.EndpointProvider {
	return []api.EndpointProvider{
		api.EndpointProviderImpl{
			HandlerValue: func(w http.ResponseWriter, r *http.Request) { s.getStatusHandler(w, r, "") },
			MethodsValue: []string{"GET"},
			RouteValue:   "/status",
		},
		api.EndpointProviderImpl{
			HandlerValue: s.componentStatusGetterHandler,
			MethodsValue: []string{"GET"},
			RouteValue:   "/{component}/status",
		},
	}

}

func (s *statusImplementation) getStatusHandler(w http.ResponseWriter, r *http.Request, section string) {
	log.Info("Got a request for the status. Making status.")
	verbose := r.URL.Query().Get("verbose") == "true"
	format := r.URL.Query().Get("format")
	var contentType string
	var res []byte

	contentType, ok := mimeTypeMap[format]

	if !ok {
		log.Warn("Got a request with invalid format parameter. Defaulting to 'text' format")
		format = "text"
		contentType = mimeTypeMap[format]
	}
	w.Header().Set("Content-Type", contentType)

	var err error
	if len(section) > 0 {
		res, err = s.GetStatusBySections([]string{section}, format, verbose)
	} else {
		res, err = s.GetStatus(format, verbose)
	}

	if err != nil {
		if format == "text" {
			http.Error(w, log.Errorf("Error getting status. Error: %v.", err).Error(), 500)
			return
		}

		utils.SetJSONError(w, log.Errorf("Error getting status. Error: %v, Status: %v", err, res), 500)
		return
	}

	w.Write(res)
}

func (s *statusImplementation) componentStatusGetterHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	component := vars["component"]
	switch component {
	case "py":
		getPythonStatus(w, r)
	default:
		s.getStatusHandler(w, r, component)
	}
}

func getPythonStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	pyStats, err := python.GetPythonInterpreterMemoryUsage()
	if err != nil {
		log.Warnf("Error getting python stats: %s\n", err) // or something like this
		http.Error(w, err.Error(), 500)
	}

	j, _ := json.Marshal(pyStats)
	w.Write(j)
}
