// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && !serverless

package configimpl

import "github.com/DataDog/datadog-agent/pkg/config/env"

// isOsHostnameUsable mirrors the same check in pkg/util/hostname/os_hostname_windows.go.
// We do not import that package directly because pkg/util/hostname/common.go pulls in
// cloud-provider clients (Azure, GCE, EC2, Docker, Kubernetes) as transitive dependencies,
// which would add ~2 MiB to the trace-agent binary for a one-line call.
func isOsHostnameUsable() bool {
	return !env.IsContainerized()
}
