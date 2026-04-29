// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package launchmetrics emits launch-time agent telemetry for serverless-init.
package launchmetrics

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
)

// DetectPlatform returns the value for the init_platform tag on
// datadog.agent.serverless_init.started. The supported managed services
// (Cloud Run, Cloud Run Jobs, Container Apps, App Service) take precedence
// via cloudService.GetOrigin(); otherwise an env-var probe identifies the
// underlying cloud, falling back to "local" when nothing matches.
//
// Detection is intentionally env-var-only: zero startup latency, no IMDS
// dependence, no fragility against IMDS hardening (hop-limit pinning,
// metadata-from-vm-only, NetworkPolicies). The trade-off is that raw VMs
// and non-IRSA-style k8s pods land on "local" instead of their underlying
// cloud.
func DetectPlatform(cloudService cloudservice.CloudService) string {
	if origin := cloudService.GetOrigin(); origin != "local" {
		return origin
	}
	return detectFromEnv()
}

func detectFromEnv() string {
	switch {
	case envSet("AWS_LAMBDA_FUNCTION_NAME"),
		envSet("ECS_CONTAINER_METADATA_URI"),
		envSet("ECS_CONTAINER_METADATA_URI_V4"),
		envSet("AWS_EXECUTION_ENV"),
		envSet("AWS_ROLE_ARN") && envSet("AWS_WEB_IDENTITY_TOKEN_FILE"):
		return "aws"
	case envSet("FUNCTION_TARGET"), envSet("GAE_SERVICE"):
		return "gcp"
	case envSet("AZURE_FUNCTIONS_ENVIRONMENT"),
		envSet("IDENTITY_ENDPOINT"),
		envSet("MSI_ENDPOINT"),
		envSet("WEBSITE_RESOURCE_GROUP"):
		return "azure"
	case envSet("FC_FUNCTION_NAME"), envSet("FC_REGION"):
		return "alibaba"
	case envSet("SCF_FUNCTIONNAME"), envSet("SCF_NAMESPACE"):
		return "tencent"
	case envSet("FN_FN_NAME"), envSet("OCI_RESOURCE_PRINCIPAL_VERSION"):
		return "oracle"
	case envSet("CE_APP"), envSet("CE_PROJECT_ID"), envSet("__OW_API_HOST"):
		return "ibm"
	}
	return "local"
}

func envSet(name string) bool {
	_, ok := os.LookupEnv(name)
	return ok
}
