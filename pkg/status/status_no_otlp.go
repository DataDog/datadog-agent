// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp

package status

// GetOTLPStatus parses the otlp pipeline and its collector info to be sent to the frontend
func GetOTLPStatus() map[string]interface{} {
	status := make(map[string]interface{})
	return status
}
