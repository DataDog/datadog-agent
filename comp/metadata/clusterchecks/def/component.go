// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusterchecks provides the clusterchecks metadata component
package clusterchecks

import (
	"net/http"
)

// team: container-platform

// Component is the component interface for clusterchecks metadata
type Component interface {
	// WritePayloadAsJSON writes the cluster checks payload as JSON to HTTP response
	WritePayloadAsJSON(w http.ResponseWriter, r *http.Request)

	// SetClusterHandler sets the cluster handler for collecting metadata
	SetClusterHandler(handler interface{})
}
