// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarderimpl exposes the event platform forwarder for netflow.
package forwarderimpl

import (
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	forwarder "github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/def"
)

// Dependencies holds the dependencies for the forwarder component.
type Dependencies struct {
	compdef.In

	Demultiplexer demultiplexer.Component
}

// GetForwarder returns the event platform forwarder from the demultiplexer.
func GetForwarder(deps Dependencies) (forwarder.Component, error) {
	return deps.Demultiplexer.GetEventPlatformForwarder()
}
