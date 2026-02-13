// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package marshal

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/indexedset"
)

type resolvConfFormatter struct {
	conns         *network.Connections
	resolvConfSet *indexedset.IndexedSet[network.ResolvConf]
}

func newResolvConfFormatter(conns *network.Connections) *resolvConfFormatter {
	return &resolvConfFormatter{
		conns:         conns,
		resolvConfSet: indexedset.New[network.ResolvConf](),
	}
}

func (f *resolvConfFormatter) FormatResolvConfIdx(nc *network.ConnectionStats, builder *model.ConnectionBuilder) {
	containerID := nc.ContainerID.Source
	resolvConf, ok := f.conns.ResolvConfs[containerID]
	if !ok {
		builder.SetResolvConfIdx(-1)
		return
	}

	builder.SetResolvConfIdx(f.resolvConfSet.Add(resolvConf))
}

func (f *resolvConfFormatter) FormatResolvConfs(builder *model.ConnectionsBuilder) {
	for _, resolvConf := range f.resolvConfSet.UniqueKeys() {
		builder.AddResolvConfs(resolvConf.Get())
	}
}
