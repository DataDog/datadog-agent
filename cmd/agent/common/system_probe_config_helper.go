// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build process

package common

import (
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

// SetupSystemProbeConfig reads the system-probe.yaml into the global config object
func SetupSystemProbeConfig(sysProbeConfFilePath string) error {
	_, err := sysconfig.Merge(sysProbeConfFilePath)
	return err
}
