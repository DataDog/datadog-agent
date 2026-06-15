// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"strings"
)

// Cloud identifies a cloud provider. Values match the cloud-prefix used in
// E2E environment names (e.g. "aws" in "aws/agent-qa").
type Cloud string

const (
	// CloudAWS is Amazon Web Services.
	CloudAWS Cloud = "aws"
	// CloudAzure is Microsoft Azure.
	CloudAzure Cloud = "az"
	// CloudGCP is Google Cloud Platform.
	CloudGCP Cloud = "gcp"
)

// internalRegistryByEnvironment maps "<cloud>/<account>" to the internal
// container registry that hosts CI-built agent images (agent-qa,
// cluster-agent-qa, dogstatsd-qa). Mirrors the defaultInternalRegistry values
// in resources/aws/environmentDefaults.go for non-Pulumi code paths.
var internalRegistryByEnvironment = map[string]string{
	"aws/sandbox":       "669783387624.dkr.ecr.us-east-1.amazonaws.com",
	"aws/agent-sandbox": "669783387624.dkr.ecr.us-east-1.amazonaws.com",
	"aws/agent-qa":      "669783387624.dkr.ecr.us-east-1.amazonaws.com",
	// aws/tse-playground has no internal registry
}

// InternalRegistry returns the internal container registry for the given
// cloud provider and the account currently active in the profile.
//
// The account is resolved from the profile's environment list: that list is
// of the form "<cloud>/<account>" (e.g. "aws/agent-qa"), with one entry per
// cloud, populated from defaults in ci_profile/local_profile and any
// E2E_ENVIRONMENTS override. We pick the entry matching `cloud` and look up
// its registry. Returns "" if no registry is configured for that account.
//
// Non-Pulumi installers (helmagent, hostagent) call this to resolve pipeline
// agent images the same way the Pulumi path does via config.Env.InternalRegistry().
func InternalRegistry(cloud Cloud) string {
	prefix := string(cloud) + "/"
	envsStr := GetProfile().EnvironmentNames()
	for env := range strings.SplitSeq(envsStr, envSep) {
		env = strings.TrimSpace(env)
		if !strings.HasPrefix(env, prefix) {
			continue
		}
		if reg, ok := internalRegistryByEnvironment[env]; ok {
			return reg
		}
	}
	return ""
}
