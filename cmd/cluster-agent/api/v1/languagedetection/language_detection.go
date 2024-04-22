// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

package languagedetection

import (
	"context"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/api"
)

const pldHandlerName = "language-detection-handler"

// InstallLanguageDetectionEndpoints installs language detection endpoints
func InstallLanguageDetectionEndpoints(ctx context.Context, r *mux.Router, wmeta workloadmeta.Component, cfg config.Component) {
	service := newLanguageDetectionHandler(wmeta, cfg)

	service.startCleanupInBackground(ctx)

	handler := api.WithLeaderProxyHandler(
		pldHandlerName,
		service.preHandler,
		service.leaderHandler,
	)
	r.HandleFunc("/languagedetection", api.WithTelemetryWrapper(pldHandlerName, handler)).Methods("POST")
}
