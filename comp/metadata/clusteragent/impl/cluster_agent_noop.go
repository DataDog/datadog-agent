// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

// Package clusteragentimpl contains a no-op implementation of the cluster-agent metatdata provider.
package clusteragentimpl

import (
	"net/http"

	clusteragent "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
)

type noopImpl struct{}

// Provides defines the output of the clusteragent metadata component
type Provides struct {
	Comp clusteragent.Component
}

// NewComponent returns a new instance of the collector component.
func NewComponent() Provides {
	dca := &noopImpl{}
	return Provides{
		Comp: dca,
	}
}

// WritePayloadAsJSON writes the payload as JSON to the response writer. NOOP implementation.
func (n *noopImpl) WritePayloadAsJSON(_ http.ResponseWriter, _ *http.Request) {}
