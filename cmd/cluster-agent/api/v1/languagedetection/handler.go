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
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
)

var (
	// statusSuccess is the value for the "status" tag that represents a successful operation
	statusSuccess = "success"
	// statusError is the value for the "status" tag that represents an error
	statusError = "error"
)

type handlerConfig struct {
	enabled       bool
	languageTTL   time.Duration
	cleanupPeriod time.Duration
}

type languageDetectionHandler struct {
	ownersLanguages OwnersLanguages
	cfg             handlerConfig
	wlm             workloadmeta.Component
}

func newLanguageDetectionHandler(wlm workloadmeta.Component, cfg config.Component) *languageDetectionHandler {
	return &languageDetectionHandler{
		cfg: handlerConfig{
			enabled:       cfg.GetBool("language_detection.enabled"),
			languageTTL:   cfg.GetDuration("cluster_agent.language_detection.cleanup.language_ttl"),
			cleanupPeriod: cfg.GetDuration("cluster_agent.language_detection.cleanup.period"),
		},
		wlm:             wlm,
		ownersLanguages: *newOwnersLanguages(),
	}
}

func (handler *languageDetectionHandler) startCleanupInBackground() {
	// Launch periodic cleanup mechanism
	go func() {
		cleanupTicker := time.NewTicker(handler.cfg.cleanupPeriod)
		for range cleanupTicker.C {
			handler.ownersLanguages.cleanExpiredLanguages(handler.wlm)
		}
	}()

	// Remove any owner when its corresponding resource is deleted
	go handler.ownersLanguages.cleanRemovedOwners(handler.wlm)
}

// preHandler is called by both leader and followers and returns true if the request should be forwarded or handled by the leader
func (handler *languageDetectionHandler) preHandler(w http.ResponseWriter, r *http.Request) bool {
	if !handler.cfg.enabled {
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
func (handler *languageDetectionHandler) leaderHandler(w http.ResponseWriter, r *http.Request) {
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

	ownersLanguagesFromRequest := getOwnersLanguages(requestData, time.Now().Add(handler.cfg.languageTTL))

	err = handler.ownersLanguages.mergeAndFlush(ownersLanguagesFromRequest, handler.wlm)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to store some (or all) languages in workloadmeta store: %s", err), http.StatusInternalServerError)
		ProcessedRequests.Inc(statusError)
		return
	}

	ProcessedRequests.Inc(statusSuccess)
	w.WriteHeader(http.StatusOK)
}
