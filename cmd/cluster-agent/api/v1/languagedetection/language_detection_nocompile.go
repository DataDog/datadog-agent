// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !kubeapiserver

package languagedetection

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/gorilla/mux"
)

// InstallLanguageDetectionEndpoints installs language detection endpoints
func InstallLanguageDetectionEndpoints(_ *mux.Router, _ workloadmeta.Component, _ config.Component) {}
