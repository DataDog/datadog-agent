// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

var mimeTypeMap = map[string]string{
	"text": "text/plain",
	"json": "application/json",
}

// SetJSONError writes a server error as JSON with the correct http error code
func SetJSONError(w http.ResponseWriter, err error, errorCode int) {
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	http.Error(w, string(body), errorCode)
	w.Header().Set("Content-Type", "application/json")
}

func (s *statusImplementation) getStatus(w http.ResponseWriter, r *http.Request, section string) {
	s.log.Info("Got a request for the status. Making status.")
	verbose := r.URL.Query().Get("verbose") == "true"
	format := r.URL.Query().Get("format")
	var contentType string
	var buff []byte

	contentType, ok := mimeTypeMap[format]

	if !ok {
		s.log.Warn("Got a request with invalid format parameter. Defaulting to 'text' format")
		format = "text"
		contentType = mimeTypeMap[format]
	}
	w.Header().Set("Content-Type", contentType)

	var err error
	if len(section) > 0 {
		buff, err = s.GetStatusBySections([]string{section}, format, verbose)
	} else {
		buff, err = s.GetStatus(format, verbose)
	}

	if err != nil {
		if format == "text" {
			http.Error(w, s.log.Errorf("Error getting status. Error: %v.", err).Error(), http.StatusInternalServerError)
			return
		}

		SetJSONError(w, s.log.Errorf("Error getting status. Error: %v, Status: %v", err, buff), http.StatusInternalServerError)
		return
	}

	w.Write(buff)
}

func (s *statusImplementation) getSections(w http.ResponseWriter, _ *http.Request) {
	s.log.Info("Got a request for the status sections.")

	w.Header().Set("Content-Type", "application/json")
	res, _ := json.Marshal(s.GetSections())
	w.Write(res)
}

func (s *statusImplementation) getSection(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	component := vars["component"]
	s.getStatus(w, r, component)
}
