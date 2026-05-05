// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// internalRegistryByEnvironment maps E2E environment names to the internal
// container registry that hosts CI-built agent images (agent-qa, cluster-agent-qa).
// This mirrors the defaultInternalRegistry values in resources/aws/environmentDefaults.go
// for non-Pulumi code paths.
var internalRegistryByEnvironment = map[string]string{
	"aws/sandbox":       "669783387624.dkr.ecr.us-east-1.amazonaws.com",
	"aws/agent-sandbox": "669783387624.dkr.ecr.us-east-1.amazonaws.com",
	"aws/agent-qa":      "669783387624.dkr.ecr.us-east-1.amazonaws.com",
	// aws/tse-playground has no internal registry
}

// InternalRegistry returns the internal container registry for the active E2E
// environment, or an empty string if no registry is configured for it.
//
// Non-Pulumi installers (helmagent, hostagent) use this to resolve pipeline
// agent images the same way the Pulumi path does via config.Env.InternalRegistry().
func InternalRegistry() string {
	envsStr, _ := GetProfile().ParamStore().GetWithDefault(parameters.Environments, "")
	for env := range strings.SplitSeq(envsStr, envSep) {
		env = strings.TrimSpace(env)
		if reg, ok := internalRegistryByEnvironment[env]; ok {
			return reg
		}
	}
	return ""
}
