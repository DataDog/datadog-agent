// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
)

// WrapWithProbeResult returns a Service that overlays ProbeResult-derived
// values on the underlying Service via GetExtraConfig. Today only
// "discovered_port" is exposed.
func WrapWithProbeResult(svc listeners.Service, r ProbeResult) listeners.Service {
	return &serviceWithProbeResult{Service: svc, result: r}
}

type serviceWithProbeResult struct {
	listeners.Service
	result ProbeResult
}

func (s *serviceWithProbeResult) GetExtraConfig(key string) (string, error) {
	if key == "discovered_port" {
		return strconv.Itoa(int(s.result.Port)), nil
	}
	return s.Service.GetExtraConfig(key)
}
