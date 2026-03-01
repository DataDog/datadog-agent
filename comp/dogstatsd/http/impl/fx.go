// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	comp "github.com/DataDog/datadog-agent/comp/def"
	def "github.com/DataDog/datadog-agent/comp/dogstatsd/http/def"
)

// Requires declares the inputs for NewComponent
type Requires struct {
	Lc       comp.Lifecycle
	Log      log.Component
	Config   config.Component
	Tagger   tagger.Component
	Hostname hostname.Component
	Demux    demultiplexer.Component
}

// Provides defines the output of this component
type Provides struct {
	Component def.Component
}

func NewComponent(req Requires) (Provides, error) {
	s := &server{
		config:   req.Config,
		log:      req.Log,
		tagger:   req.Tagger,
		hostname: req.Hostname,

		out: req.Demux.Serializer(),
	}

	req.Lc.Append(comp.Hook{
		OnStart: s.start,
		OnStop:  s.stop,
	})

	return Provides{
		Component: s,
	}, nil
}
