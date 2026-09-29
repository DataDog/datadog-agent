// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// spProcessJobIDsEndpoint is the system-probe GPU module endpoint that serves the
// training job identifiers configured as environment variables (gpu.jobs).
const spProcessJobIDsEndpoint = "/process-job-ids"

// NewSystemProbeJobIDReader returns a JobIDReader that gets the identifiers through
// system-probe, which has access to the host procfs and the privileges to read the
// environment of any process.
func NewSystemProbeJobIDReader() JobIDReader {
	return newSystemProbeJobIDReader(nvidia.NewSystemProbeClient())
}

func newSystemProbeJobIDReader(client *sysprobeclient.CheckClient) JobIDReader {
	return func(pid int) (model.ProcessJobIDs, error) {
		request := model.ProcessJobIDsRequest{PID: pid}
		return sysprobeclient.Post[model.ProcessJobIDs](client, spProcessJobIDsEndpoint, request, sysconfig.GPUMonitoringModule)
	}
}
