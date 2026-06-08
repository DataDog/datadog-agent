// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !kubeapiserver

package languagedetection

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// InstallLanguageDetectionEndpoints installs language detection endpoints
func InstallLanguageDetectionEndpoints(_ context.Context, _ *http.ServeMux, _ workloadmeta.Component, _ config.Component) {
}
