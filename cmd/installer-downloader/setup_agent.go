// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !setup_databricks && !setup_emr && !setup_dataproc

package main

import (
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/defaultscript"
)

func run(s *common.Setup) error {
	return defaultscript.SetupDefaultScript(s)
}
