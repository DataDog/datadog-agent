// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !kubeapiserver

package series

import (
	"context"

	"github.com/gorilla/mux"

	config "github.com/DataDog/datadog-agent/comp/core/config/def"
)

// InstallNodeMetricsEndpoints installs node metrics collection endpoints
func InstallNodeMetricsEndpoints(_ context.Context, _ *mux.Router, _ config.Component) {
}
