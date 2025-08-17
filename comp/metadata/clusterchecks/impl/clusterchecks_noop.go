// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !clusterchecks

// Package clusterchecksimpl contains a no-op implementation of the clusterchecks metadata provider.
package clusterchecksimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"
	"github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

type noopImpl struct{}

// Requires defines the dependencies for the clusterchecks metadata component (no-op)
type Requires struct {
	Log        log.Component
	Conf       config.Component
	Serializer serializer.MetricSerializer
}

// Provides defines the output of the clusterchecks metadata component (no-op)
type Provides struct {
	Comp     clusterchecksmetadata.Component
	Provider runnerimpl.Provider
}

// NewComponent returns a new instance of the clusterchecks component (no-op implementation).
func NewComponent(_ Requires) Provides {
	comp := &noopImpl{}
	return Provides{
		Comp:     comp,
		Provider: runnerimpl.NewProvider(nil),
	}
}

// WritePayloadAsJSON is a no-op when cluster checks are not compiled
func (n *noopImpl) WritePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message": "cluster checks not compiled"}`))
}

// SetClusterHandler is a no-op when cluster checks are not compiled
func (n *noopImpl) SetClusterHandler(_ interface{}) {
	// No-op
}
