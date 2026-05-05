// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func candidatePorts(hints []int, exposed []workloadmeta.ContainerPort) []uint16 {
	exposedSet := make(map[uint16]struct{}, len(exposed))
	for _, p := range exposed {
		exposedSet[uint16(p.Port)] = struct{}{}
	}

	out := make([]uint16, 0, len(exposed))
	seen := make(map[uint16]struct{}, len(exposed))

	for _, h := range hints {
		p := uint16(h)
		if _, ok := exposedSet[p]; !ok {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		out = append(out, p)
		seen[p] = struct{}{}
	}

	for _, p := range exposed {
		port := uint16(p.Port)
		if _, dup := seen[port]; dup {
			continue
		}
		out = append(out, port)
		seen[port] = struct{}{}
	}

	return out
}
