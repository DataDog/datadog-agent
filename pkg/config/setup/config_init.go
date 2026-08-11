// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package setup

func initConfig() {
	cfg := GlobalConfigBuilder()
	InitConfig(cfg)

	sysprobe := GlobalSystemProbeConfigBuilder()
	InitSystemProbeConfig(sysprobe)
}

func fixupInitConfig() {
	ddcfg := Datadog()
	fixupInitCommonConfigComponents(ddcfg)
	fixupInitFullAgentOnlyComponents(ddcfg)
}

// fixupPostBuildConfig runs fixups that need to read config values, so they must run after
// BuildSchema() has marked the config ready for use (unlike fixupInitConfig, which only
// registers override funcs and declares defaults before the config is ready).
func fixupPostBuildConfig() {
	fixupInitSystemProbe(SystemProbe())
}
