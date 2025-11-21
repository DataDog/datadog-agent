// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package compliance

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
)

func createResolverOptions(hostname string) *compliance.ResolverOptions {
	return &compliance.ResolverOptions{
		Hostname:           hostname,
		HostRoot:           os.Getenv("HOST_ROOT"),
		DockerProvider:     compliance.DefaultDockerProvider,
		LinuxAuditProvider: compliance.DefaultLinuxAuditProvider,
	}
}

func newSysProbeClient(address string) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 30 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", address)
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}
}
