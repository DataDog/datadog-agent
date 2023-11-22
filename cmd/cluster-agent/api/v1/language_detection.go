// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/languagedetection"
	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"

	"github.com/gorilla/mux"
	"google.golang.org/protobuf/proto"
)

// InstallLanguageDetectionEndpoints installs language detection endpoints
func InstallLanguageDetectionEndpoints(r *mux.Router, wmeta workloadmeta.Component) {
	r.HandleFunc("/languagedetection", api.WithTelemetryWrapper("postDetectedLanguages",
		func(w http.ResponseWriter, r *http.Request) {
			postDetectedLanguages(w, r, wmeta)
		})).Methods("POST")
}

func postDetectedLanguages(w http.ResponseWriter, r *http.Request, wmeta workloadmeta.Component) {
	if !config.Datadog.GetBool("language_detection.enabled") {
		languagedetection.ErrorResponses.Inc()
		http.Error(w, "Language detection feature is disabled on the cluster agent", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		languagedetection.ErrorResponses.Inc()
		return
	}

	// Create a new instance of the protobuf message type
	requestData := &pbgo.ParentLanguageAnnotationRequest{}

	// Unmarshal the request body into the protobuf message
	err = proto.Unmarshal(body, requestData)
	if err != nil {
		http.Error(w, "Failed to unmarshal request body", http.StatusBadRequest)
		languagedetection.ErrorResponses.Inc()
		return
	}

	lp, err := languagedetection.NewLanguagePatcher(wmeta)

	if err != nil {
		http.Error(w, "Failed to get k8s apiserver client", http.StatusInternalServerError)
		languagedetection.ErrorResponses.Inc()
		return
	}

	// Answer before patching
	languagedetection.OkResponses.Inc()
	w.WriteHeader(http.StatusOK)

	// Patch annotations to deployments
	lp.PatchAllOwners(requestData)
}
