// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"

	"github.com/gorilla/mux"
	"google.golang.org/protobuf/proto"
)

const pldHandlerName = "language-detection-handler"

var (
	// statusSuccess is the value for the "status" tag that represents a successful operation
	statusSuccess = "success"
	// statusError is the value for the "status" tag that represents an error
	statusError = "error"
)

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

var ownersLanguages OwnersLanguages
var languageTTL time.Duration
var ownersLanguagesOnce sync.Once

func loadOwnersLanguages(wlm workloadmeta.Component) *OwnersLanguages {
	ownersLanguagesOnce.Do(func() {
		ownersLanguages = *newOwnersLanguages()
		languageTTL = config.Datadog.GetDuration("cluster_agent.language_detection.cleanup.language_ttl")
		cleanupPeriod := config.Datadog.GetDuration("cluster_agent.language_detection.cleanup.period")

		// Launch periodic cleanup mechanism
		go func() {
			cleanupTicker := time.NewTicker(cleanupPeriod)
			for range cleanupTicker.C {
				ownersLanguages.cleanExpiredLanguages(wlm)
			}
		}()

		// Remove any owner when its corresponding resource is deleted
		go ownersLanguages.cleanRemovedOwners(wlm)
	})
	return &ownersLanguages
}

// preHandler is called by both leader and followers and returns true if the request should be forwarded or handled by the leader
func preHandler(w http.ResponseWriter, r *http.Request) bool {
	if !config.Datadog.GetBool("language_detection.enabled") {
		ProcessedRequests.Inc(statusError)
		http.Error(w, "Language detection feature is disabled on the cluster agent", http.StatusServiceUnavailable)
		return false
	}

	// Reject if no body
	if r.Body == nil {
		ProcessedRequests.Inc(statusError)
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
		ProcessedRequests.Inc(statusError)
		return
	}

	// Create a new instance of the protobuf message type
	requestData := &pbgo.ParentLanguageAnnotationRequest{}

	// Unmarshal the request body into the protobuf message
	err = proto.Unmarshal(body, requestData)
	if err != nil {
		http.Error(w, "Failed to unmarshal request body", http.StatusBadRequest)
		ProcessedRequests.Inc(statusError)
		return
	}

	ownersLanguagesFromRequest := getOwnersLanguages(requestData, time.Now().Add(languageTTL))

	ownersLanguage := loadOwnersLanguages(wlm)
	err = ownersLanguage.mergeAndFlush(ownersLanguagesFromRequest, wlm)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to store some (or all) languages in workloadmeta store: %s", err), http.StatusInternalServerError)
		ProcessedRequests.Inc(statusError)
		return
	}

	ProcessedRequests.Inc(statusSuccess)
	w.WriteHeader(http.StatusOK)
}
