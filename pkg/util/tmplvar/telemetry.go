// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tmplvar

import (
	"strconv"
	"strings"
	"sync"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

var agentEnvVarResolutions = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	"autodiscovery",
	"agent_env_var_resolutions",
	[]string{"env_var", "allowed"},
	"Count of %%env_*%% template variable resolutions of env vars bound to Agent settings, by env var and whether resolution was allowed.",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

var agentEnvVars = sync.OnceValue(func() map[string]struct{} {
	envVars := pkgconfigsetup.Datadog().GetEnvVars()
	set := make(map[string]struct{}, len(envVars))
	for _, envVar := range envVars {
		set[strings.ToUpper(envVar)] = struct{}{}
	}
	return set
})

func recordAgentEnvVarResolution(envVar string, allowed bool) {
	envVar = strings.ToUpper(envVar)
	if _, ok := agentEnvVars()[envVar]; !ok {
		return
	}
	agentEnvVarResolutions.Inc(envVar, strconv.FormatBool(allowed))
}
