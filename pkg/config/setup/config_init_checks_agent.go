// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build checks_agent

package setup

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
)

func initConfig() {
	datadog = nodetreemodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))          // nolint: forbidigo // legit use case
	systemProbe = nodetreemodel.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

	InitConfig(Datadog())
	InitSystemProbeConfig(SystemProbe())

	datadog.BuildSchema()
}
