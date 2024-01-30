// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

// Package languagedetection defines and implements the language detection handler endpoint
package languagedetection

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"

	"github.com/gorilla/mux"
	"google.golang.org/protobuf/proto"
)

const pldHandlerName = "language-detection-handler"

// InstallLanguageDetectionEndpoints installs language detection endpoints
func InstallLanguageDetectionEndpoints(r *mux.Router, wmeta workloadmeta.Component) {
	handler := api.WithLeaderProxyHandler(
		pldHandlerName,
		preHandler,
		func(w http.ResponseWriter, r *http.Request) {
			leaderHandler(w, r, wmeta)
		},
	)
	r.HandleFunc("/languagedetection", api.WithTelemetryWrapper(pldHandlerName, handler)).Methods("POST")
}

var ownersLanguage OwnersLanguages
var ownersLanguagesOnce sync.Once

func loadOwnersLanguages() *OwnersLanguages {
	ownersLanguagesOnce.Do(func() {
		ownersLanguage = *newOwnersLanguages()
	})
	return &ownersLanguage
}

// preHandler is called by both leader and followers and returns true if the request should be forwarded or handled by the leader
func preHandler(w http.ResponseWriter, r *http.Request) bool {
	if !config.Datadog.GetBool("language_detection.enabled") {
		ErrorResponses.Inc()
		http.Error(w, "Language detection feature is disabled on the cluster agent", http.StatusServiceUnavailable)
		return false
	}

	// Reject if no body
	if r.Body == nil {
		ErrorResponses.Inc()
		http.Error(w, "Request body is empty", http.StatusBadRequest)
		return false
	}

	return true
}

// leaderHandler is called only by the leader and used to patch the annotations
func leaderHandler(w http.ResponseWriter, r *http.Request, wlm workloadmeta.Component) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		ErrorResponses.Inc()
		return
	}

	// Create a new instance of the protobuf message type
	requestData := &pbgo.ParentLanguageAnnotationRequest{}

	// Unmarshal the request body into the protobuf message
	err = proto.Unmarshal(body, requestData)
	if err != nil {
		http.Error(w, "Failed to unmarshal request body", http.StatusBadRequest)
		ErrorResponses.Inc()
		return
	}

	ownersLanguagesFromRequest := getOwnersLanguages(requestData)
	ownersLanguages := loadOwnersLanguages()
	ownersLanguages.merge(ownersLanguagesFromRequest)

	err = ownersLanguages.clean(wlm)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to store some (or all) languages in workloadmeta store: %s", err), http.StatusInternalServerError)
		ErrorResponses.Inc()
		return
	}

	OkResponses.Inc()
	w.WriteHeader(http.StatusOK)
}
