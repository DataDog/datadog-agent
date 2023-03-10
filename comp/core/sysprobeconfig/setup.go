// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobeconfig

import (
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

func setupConfig(deps dependencies) (*sysconfig.Config, error) {
	confFilePath := deps.Params.sysProbeConfFilePath
	return sysconfig.New(confFilePath)
}
