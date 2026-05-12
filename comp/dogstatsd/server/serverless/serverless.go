// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverless provides the serverless dogstatsd server constructor.
// External packages should import this package instead of the impl package
// to avoid depending on internal implementation details.
package serverless

import (
	serverdef "github.com/DataDog/datadog-agent/comp/dogstatsd/server/def"
	serverimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/server/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// team: agent-metric-pipelines

// NewServerlessServer creates and starts a new serverless dogstatsd server.
func NewServerlessServer(demux aggregator.Demultiplexer, extraTags []string) (serverdef.ServerlessDogstatsd, error) {
	return serverimpl.NewServerlessServer(demux, extraTags)
}
