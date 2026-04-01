// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentimpl

import (
	"strings"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
)

const (
	apmCgroupV2CheckID   = "apm-cgroup-v2-container-id"
	apmCgroupV2CheckName = "APM Container ID Resolution on cgroup v2"
	apmCgroupV2IssueID   = "apm-cgroup-v2-container-tags-missing"
)

// checkCgroupV2Health reports a health platform issue if the cgroup reader fails to
// initialize, which prevents APM from resolving container IDs on cgroup v2 + Kubernetes.
func checkCgroupV2Health(conf *tracecfg.AgentConfig, hp healthplatformdef.Component) {
	if hp == nil {
		return
	}

	var hostPrefix string
	if strings.HasPrefix(conf.ContainerProcRoot, "/host") {
		hostPrefix = "/host"
	}

	_, err := cgroups.NewReader(
		cgroups.WithCgroupV1BaseController("memory"),
		cgroups.WithProcPath(conf.ContainerProcRoot),
		cgroups.WithHostPrefix(hostPrefix),
		cgroups.WithReaderFilter(cgroups.ContainerFilter),
	)
	if err != nil {
		_ = hp.ReportIssue(apmCgroupV2CheckID, apmCgroupV2CheckName, &healthplatformpayload.IssueReport{
			IssueId: apmCgroupV2IssueID,
			Context: map[string]string{"error": err.Error()},
			Tags:    []string{"apm", "cgroup", "kubernetes"},
		})
		return
	}

	// Clear any previously reported issue on successful detection.
	_ = hp.ReportIssue(apmCgroupV2CheckID, apmCgroupV2CheckName, nil)
}
