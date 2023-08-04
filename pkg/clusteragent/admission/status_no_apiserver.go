// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

package admission

// GetStatus returns status info for the secret and webhook controllers.
func GetStatus(apiCl interface{}) map[string]interface{} {
	status := make(map[string]interface{})
	status["Disabled"] = "The admission controller is not compiled-in"
	return status
}
