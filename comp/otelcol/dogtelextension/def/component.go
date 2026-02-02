// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dogtelextension provides a minimal set of Datadog agent functionalities for DDOT in standalone mode
package dogtelextension

import (
	"go.opentelemetry.io/collector/extension"
)

// team: opentelemetry-agent

// Component provides Datadog agent functionalities for OTel collector including:
// - Host metadata submission
// - Remote tagger gRPC server
// - Secrets resolution
// - Workload detection integration
type Component interface {
	extension.Extension // Implement OTel Extension lifecycle
}
