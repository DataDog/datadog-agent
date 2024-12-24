// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package statusimpl implements the status component interface
package statusimpl

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var mimeTypeMap = map[string]string{
	"text": "text/plain",
	"json": "application/json",
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

	if len(buff) != 0 {
		// scrub status output
		s := scrubber.DefaultScrubber
		var e error
		if format == "json" {
			buff, e = s.ScrubJSON(buff)
		} else {
			buff, e = s.ScrubBytes(buff)
		}
		if e != nil {
			buff = []byte("[REDACTED] - failure to clean the message")
		}
	}

	if err != nil {
		var errorMsg string
		var scrubbedMsg []byte
		var scrubOperationErr error

		if format == "text" {
			errorMsg = fmt.Sprintf("Error getting status. Error: %v.", err)
			scrubbedMsg, scrubOperationErr = scrubber.DefaultScrubber.ScrubBytes([]byte(errorMsg))
		} else {
			errorMsg = fmt.Sprintf("Error getting status. Error: %v, Status: %v", err, buff)
			body, _ := json.Marshal(map[string]string{"error": errorMsg})
			scrubbedMsg, scrubOperationErr = scrubber.DefaultScrubber.ScrubJSON(body)
		}

		if scrubOperationErr != nil {
			scrubbedMsg = []byte("[REDACTED] failed to clean error")
		}

		http.Error(w, s.log.Error(string(scrubbedMsg)).Error(), http.StatusInternalServerError)
		if format == "json" {
			w.Header().Set("Content-Type", "application/json")
		}
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
