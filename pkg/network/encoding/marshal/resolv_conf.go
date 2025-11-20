// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package marshal

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
)

type resolvConfFormatter struct {
	conns         *network.Connections
	resolvConfSet map[network.ResolvConf]int
}

func newResolvConfFormatter(conns *network.Connections) *resolvConfFormatter {
	return &resolvConfFormatter{
		conns:         conns,
		resolvConfSet: make(map[network.ResolvConf]int),
	}
}

func (f *resolvConfFormatter) FormatResolvConfIdx(nc *network.ConnectionStats, builder *model.ConnectionBuilder) {
	containerID := nc.ContainerID.Source
	resolvConf, ok := f.conns.ResolvConfs[containerID]
	if !ok {
		return
	}

	resolvConfIdx, ok := f.resolvConfSet[resolvConf]
	if !ok {
		resolvConfIdx = len(f.resolvConfSet) + 1
		f.resolvConfSet[resolvConf] = resolvConfIdx
	}

	builder.SetResolvConfIdx(int32(resolvConfIdx))
}

func (f *resolvConfFormatter) FormatResolvConfs(builder *model.ConnectionsBuilder) {
	resolvConfList := make([]string, len(f.resolvConfSet)+1)
	for resolvConf, idx := range f.resolvConfSet {
		resolvConfList[idx] = resolvConf.Get()
	}

	for _, resolvConf := range resolvConfList {
		builder.AddResolvConfs(resolvConf)
	}
}
