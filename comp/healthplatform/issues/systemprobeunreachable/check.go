// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package systemprobeunreachable

import (
	"net"
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	defaultSocketPath = "/var/run/sysprobe/sysprobe.sock"
	dialTimeout       = 2 * time.Second
)

// BuiltInStartupHealthCheck returns nil if neither NPM nor USM is enabled so the check is
// never registered. Otherwise it returns a startup check that dials the system-probe socket.
func (m *systemProbeUnreachableModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	sysCfg := pkgconfigsetup.SystemProbe()
	if !sysCfg.GetBool("network_config.enabled") && !sysCfg.GetBool("service_monitoring_config.enabled") {
		return nil
	}
	return &runnerdef.BuiltInHealthCheck{
		Source: "system-probe",
		Fn:     Check,
	}
}

// Check dials the system-probe socket and returns an IssueReport if unreachable.
// It assumes NPM or USM is enabled — callers must gate on that before registering.
func Check() ([]runnerdef.IssueReport, error) {
	sysCfg := pkgconfigsetup.SystemProbe()
	socketPath := sysCfg.GetString("system_probe_config.sysprobe_socket")
	if socketPath == "" {
		socketPath = defaultSocketPath
	}

	conn, err := net.DialTimeout("unix", socketPath, dialTimeout)
	if err == nil {
		conn.Close()
		return nil, nil
	}

	return []runnerdef.IssueReport{
		{
			IssueID:   IssueID,
			IssueName: IssueName,
			Context: map[string]string{
				"socket": socketPath,
			},
		},
	}, nil
}
