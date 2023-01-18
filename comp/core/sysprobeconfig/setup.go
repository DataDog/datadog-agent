// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sysprobeconfig

import (
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func setupConfig(deps dependencies) (*config.Warnings, error) {
	confFilePath := deps.Params.sysProbeConfFilePath
	_, err := sysconfig.New(confFilePath)
	return nil, err
}
