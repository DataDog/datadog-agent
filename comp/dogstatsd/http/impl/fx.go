// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	comp "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/http/def"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
)

// Requires declares the inputs for NewComponent
type Requires struct {
	Lc     comp.Lifecycle
	Log    log.Component
	Config config.Component
	Demux  demultiplexer.Component
}

// Provides defines the output of this component
type Provides struct {
	Component def.Component
}

func NewComponent(req Requires) (Provides, error) {
	s := newServer(req.Config, req.Log, req.Demux.Serializer())

	req.Lc.Append(comp.Hook{
		OnStart: s.start,
		OnStop:  s.stop,
	})

	return Provides{
		Component: s,
	}, nil
}
