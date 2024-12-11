// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import installer "github.com/DataDog/datadog-agent/test/new-e2e/pkg/components/datadog-installer"

// RemoteDatadogInstaller represents a Datadog Installer on a remote machine
type RemoteDatadogInstaller struct {
	installer.Output
}
