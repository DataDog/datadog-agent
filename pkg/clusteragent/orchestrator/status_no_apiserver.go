// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import "context"

// GetStatus returns status info for the orchestrator explorer.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func GetStatus(_ context.Context, _ interface{}) map[string]interface{} {
	status := make(map[string]interface{})
	status["Disabled"] = "The orchestrator explorer is not compiled-in"
	return status
}
