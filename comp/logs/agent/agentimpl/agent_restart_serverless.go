// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build serverless

package agentimpl

// smartHTTPRestart is a no-op for serverless builds
func (a *logAgent) smartHTTPRestart() {
	// No-op: serverless agents don't need HTTP retry functionality
}

// stopHTTPRetry is a no-op for serverless builds
func (a *logAgent) stopHTTPRetry() {
	// No-op: serverless agents don't need HTTP retry functionality
}
