// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless && !checks_agent

package setup

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/DataDog/datadog-agent/pkg/config/teeconfig"
	viperconfig "github.com/DataDog/datadog-agent/pkg/config/viperconfig"
)

func initConfig() {
	// Configure Datadog global configuration
	envvar := os.Getenv("DD_CONF_NODETREEMODEL")
	// Possible values for DD_CONF_NODETREEMODEL:
	// - "enable":    Use the nodetreemodel for the config, instead of viper
	// - "tee":       Construct both viper and nodetreemodel. Write to both, only read from viper
	// - "unmarshal": Use viper for the config but the reflection based version of UnmarshalKey which used some of
	//                nodetreemodel internals
	// - other:       Use viper for the config
	if envvar == "enable" {
		datadog = nodetreemodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	} else if envvar == "tee" {
		viperConfig := viperconfig.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))      // nolint: forbidigo // legit use case
		nodetreeConfig := nodetreemodel.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
		datadog = teeconfig.NewTeeConfig(viperConfig, nodetreeConfig)
	} else {
		datadog = viperconfig.NewConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	}

	systemProbe = viperconfig.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

	// Configuration defaults

	InitConfig(Datadog())
	InitSystemProbeConfig(SystemProbe())

	datadog.BuildSchema()
}
