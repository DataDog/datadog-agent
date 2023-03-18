// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build !kubeapiserver

package externalmetrics

// GetStatus returns various information about the datadog client(s)
func GetStatus() map[string]interface{} {
	return map[string]interface{}{}
}
