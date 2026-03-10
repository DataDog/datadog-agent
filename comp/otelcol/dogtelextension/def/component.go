// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogtelextension provides Datadog agent functionalities for OTel collector
package dogtelextension

import (
	"go.opentelemetry.io/collector/extension"
)

// team: opentelemetry

// Component provides Datadog agent functionalities for OTel collector including:
// - Host metadata submission
// - Remote tagger gRPC server
// - Secrets resolution (conditional)
// - Workload detection integration
type Component interface {
	extension.Extension // Implement OTel Extension lifecycle

	// GetTaggerServerPort returns the port where tagger gRPC server is listening.
	// Returns 0 if server is not started.
	GetTaggerServerPort() int
}
