// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package inventory populates serverless-init-specific fields in the
// inventoryagent payload. The agentmetadata EPRW decoder (SVLS-9607) reads
// these fields to write a serverless_init_agent REDAPL record in addition to
// (or instead of) the standard datadog_agent record.
//
// All fields are prefixed "serverless_" so the decoder can identify them and
// route them to the correct table. Fields that are empty or unknown are
// omitted via the Component.Set no-op path.
package inventory

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgversion "github.com/DataDog/datadog-agent/pkg/version"
)

// SetInventoryFields populates serverless-init-specific metadata in the
// inventoryagent component. Call once in run() before ForceCollect() so all
// fields are present in the first payload sent per container lifecycle.
//
// Fields map to the serverless_init_agent REDAPL table defined in SVLS-9604.
// The EPRW agentmetadata decoder (SVLS-9607) extracts fields prefixed with
// "serverless_" and writes them to that table.
func SetInventoryFields(ia inventoryagent.Component, cs cloudservice.CloudService, mc mode.Conf) {
	origin := cs.GetOrigin()

	ia.Set("serverless_cloud_provider", cloudProviderFromOrigin(origin))
	ia.Set("serverless_workload_type", workloadTypeFromOrigin(origin))
	ia.Set("serverless_origin", origin)
	ia.Set("serverless_deployment_model", deploymentModelFromConf(mc))
	ia.Set("serverless_agent_commit", pkgversion.Commit)
	ia.Set("serverless_runtime", detectRuntime())

	if name := resourceNameFromOrigin(origin); name != "" {
		ia.Set("serverless_resource_name", name)
	}
	if !mc.SidecarMode {
		ia.Set("serverless_wrapped_command", strings.Join(os.Args[1:], " "))
	}

	cfg := pkgconfigsetup.Datadog()
	ia.Set("serverless_dd_site", cfg.GetString("site"))
	ia.Set("serverless_logs_enabled", cfg.GetBool("logs_enabled"))
	ia.Set("serverless_apm_enabled", cfg.GetBool("apm_config.enabled"))

	// Category B — user-set; omit if absent so REDAPL stores NULL not empty string
	if v := os.Getenv("DD_ENV"); v != "" {
		ia.Set("serverless_dd_env", v)
	}
	if v := os.Getenv("DD_SERVICE"); v != "" {
		ia.Set("serverless_dd_service", v)
	}
	if v := os.Getenv("DD_VERSION"); v != "" {
		ia.Set("serverless_dd_version", v)
	}
	if v := firstEnv("DD_TRACE_VERSION", "DD_TRACER_VERSION"); v != "" {
		ia.Set("serverless_tracer_version", v)
	}

	// Azure-only identity fields
	if v := os.Getenv(cloudservice.AzureSubscriptionIdEnvVar); v != "" {
		ia.Set("serverless_subscription_id", v)
	}
	if v := os.Getenv(cloudservice.AzureResourceGroupEnvVar); v != "" {
		ia.Set("serverless_resource_group", v)
	}
}

// cloudProviderFromOrigin maps the cloudservice origin string to a canonical
// cloud provider name used in the serverless_init_agent table.
func cloudProviderFromOrigin(origin string) string {
	switch origin {
	case cloudservice.CloudRunOrigin, cloudservice.CloudRunJobsOrigin:
		return "gcp"
	case cloudservice.ContainerAppOrigin, cloudservice.AppServiceOrigin:
		return "azure"
	default:
		return origin
	}
}

// workloadTypeFromOrigin maps the cloudservice origin to a workload type enum
// value matching the serverless_init_agent table schema (SVLS-9604).
func workloadTypeFromOrigin(origin string) string {
	switch origin {
	case cloudservice.CloudRunOrigin:
		// Cloud Run Functions gen2 run on Cloud Run but expose FUNCTION_TARGET.
		if os.Getenv("FUNCTION_TARGET") != "" {
			return "cloud_run_function"
		}
		return "cloud_run_service"
	case cloudservice.CloudRunJobsOrigin:
		return "cloud_run_job"
	case cloudservice.ContainerAppOrigin:
		return "container_app"
	case cloudservice.AppServiceOrigin:
		return "app_service"
	default:
		return origin
	}
}

// resourceNameFromOrigin returns the human-readable resource name for Fleet
// display by reading the platform-injected env var for the detected origin.
func resourceNameFromOrigin(origin string) string {
	switch origin {
	case cloudservice.CloudRunOrigin, cloudservice.CloudRunJobsOrigin:
		return os.Getenv(cloudservice.ServiceNameEnvVar) // K_SERVICE
	case cloudservice.ContainerAppOrigin:
		return os.Getenv(cloudservice.ContainerAppNameEnvVar) // CONTAINER_APP_NAME
	case cloudservice.AppServiceOrigin:
		return os.Getenv(cloudservice.WebsiteName) // WEBSITE_SITE_NAME
	}
	return ""
}

func deploymentModelFromConf(mc mode.Conf) string {
	if mc.SidecarMode {
		return "sidecar"
	}
	return "init-container"
}

// detectRuntime infers the language runtime from well-known base-image env vars.
func detectRuntime() string {
	for prefix, env := range map[string]string{
		"python": "PYTHON_VERSION",
		"node":   "NODE_VERSION",
		"java":   "JAVA_VERSION",
		"dotnet": "DOTNET_VERSION",
		"ruby":   "RUBY_VERSION",
	} {
		if v := os.Getenv(env); v != "" {
			return prefix + v
		}
	}
	return ""
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
