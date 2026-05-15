// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package agentimpl

import (
	"strings"

	apmcgroupv2 "github.com/DataDog/datadog-agent/comp/healthplatform/issues/apm-cgroup-v2-container-tags"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// checkCgroupV2Health reports a health platform issue if the cgroup reader fails to
// initialize, which prevents APM from resolving container IDs on cgroup v2 + Kubernetes.
func checkCgroupV2Health(conf *tracecfg.AgentConfig, hp option.Option[storedef.Component]) {
	store, ok := hp.Get()
	if !ok {
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
		_ = store.ReportIssue(storedef.IssueReport{
			IssueID:   apmcgroupv2.IssueID,
			IssueType: apmcgroupv2.IssueID,
			Source:    "apm",
			Context:   map[string]string{"error": err.Error()},
			Tags:      []string{"apm", "cgroup", "kubernetes"},
		})
		return
	}

	// Clear any previously reported issue on successful initialization.
	store.ResolveIssue(apmcgroupv2.IssueID)
}
