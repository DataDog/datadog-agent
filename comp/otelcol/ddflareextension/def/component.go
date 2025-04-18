// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ddflareextension defines the OpenTelemetry Extension component.
package ddflareextension

import (
	"net/http"

	"go.opentelemetry.io/collector/extension"
)

// team: opentelemetry-agent

// Component specifies the interface implemented by the extension module.
type Component interface {
	extension.Extension // Embed base Extension for common functionality.
	http.Handler
}
