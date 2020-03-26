// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

package containers

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

// SetMetrics stores results from a ContainerMetrics to the embedded struct inside Container
func (ctn *Container) SetMetrics(ctnMetrics *metrics.ContainerMetrics) {
	if ctnMetrics == nil {
		return
	}

	ctn.CPU = ctnMetrics.CPU
	ctn.IO = ctnMetrics.IO
	ctn.Memory = ctnMetrics.Memory
}

// SetLimits stores results from a ContainerLimits to a Container
func (ctn *Container) SetLimits(ctnLimits *metrics.ContainerLimits) {
	if ctnLimits == nil {
		return
	}

	ctn.CPULimit = ctnLimits.CPULimit
	ctn.MemLimit = ctnLimits.MemLimit
	ctn.ThreadLimit = ctnLimits.ThreadLimit
}
