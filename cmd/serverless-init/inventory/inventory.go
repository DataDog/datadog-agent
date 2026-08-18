// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package inventory populates serverless-init-specific fields in the
// inventoryagent payload. The agentmetadata EPRW decoder (SVLS-9607) reads
// these fields to write a serverless_init_agent REDAPL record in addition to
// the standard datadog_agent record.
//
// Routing: the decoder matches the inventory "flavor" field against the literal
// "serverless-init". main.go calls flavor.SetFlavor("serverless-init") so the
// per-flavor write branch fires. Field keys are UNPREFIXED and match the
// decoder's stringFields allowlist exactly. Fields that are empty or unknown
// are omitted via the Component.Set no-op path so REDAPL stores NULL, not a
// sentinel.
package inventory

import (
	"fmt"
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
// The EPRW agentmetadata decoder (SVLS-9607) reads the unprefixed keys below
// from the agent_metadata map and writes them to that table. Required identity
// fields (resource_id, uuid, resource_name, workload_type) MUST be present or
// the decoder rejects the per-flavor write; the standard datadog_agent write
// still proceeds.
func SetInventoryFields(ia inventoryagent.Component, cs cloudservice.CloudService, mc mode.Conf) {
	origin := cs.GetOrigin()
	tags := cs.GetTags()

	// --- Required identity (RFC primary key: resource_id + uuid) ---
	// uuid is the Agent host UUID already present in the inventory payload; the
	// decoder reads it from amd["uuid"]. resource_id is the full cloud resource
	// identifier derived from cloudservice tags so it joins to crawler resources.
	if rid := resourceIDFromTags(origin, tags); rid != "" {
		ia.Set("resource_id", rid)
	}
	if name := resourceNameFromOrigin(origin, tags); name != "" {
		ia.Set("resource_name", name)
	}
	if wt := workloadTypeFromOrigin(origin); wt != "" {
		ia.Set("workload_type", wt)
	}

	// --- Nullable serverless-init fields (canonical names) ---
	// agent_version_base and serverless_init_version are both the serverless-init
	// image version, which is the Agent version it ships with.
	ia.Set("agent_version_base", pkgversion.AgentVersion)
	ia.Set("agent_commit", pkgversion.Commit)
	ia.Set("serverless_init_version", pkgversion.AgentVersion)
	ia.Set("deployment_model", deploymentModelFromConf(mc))
	ia.Set("runtime", detectRuntime())
	if !mc.SidecarMode {
		ia.Set("wrapped_command", strings.Join(os.Args[1:], " "))
	}

	cfg := pkgconfigsetup.Datadog()
	ia.Set("dd_site", cfg.GetString("site"))

	// Category B — user-set; omit if absent so REDAPL stores NULL not empty string
	if v := os.Getenv("DD_ENV"); v != "" {
		ia.Set("dd_env", v)
	}
	if v := os.Getenv("DD_SERVICE"); v != "" {
		ia.Set("dd_service", v)
	}
	if v := os.Getenv("DD_VERSION"); v != "" {
		ia.Set("dd_version", v)
	}

	// Cloud-provider-specific nullable identity fields (derived, not stored as
	// cloud_provider per RFC — provider is implied by resource_id's scheme).
	switch cloudProviderFromOrigin(origin) {
	case "gcp":
		if v := gcpProjectIDFromTags(tags); v != "" {
			ia.Set("gcp_project_id", v)
		}
	case "azure":
		if v := os.Getenv(cloudservice.AzureSubscriptionIdEnvVar); v != "" {
			ia.Set("azure_subscription_id", v)
		}
		if v := os.Getenv(cloudservice.AzureResourceGroupEnvVar); v != "" {
			ia.Set("azure_resource_group", v)
		}
	}
}

// cloudProviderFromOrigin maps the cloudservice origin string to a canonical
// cloud provider name. Used only to gate provider-specific fields; it is NOT
// emitted as a field (RFC: provider is derived from resource_id's scheme).
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
// value matching the RFC canonical vocabulary.
func workloadTypeFromOrigin(origin string) string {
	switch origin {
	case cloudservice.CloudRunOrigin:
		// Cloud Functions Gen2 run on Cloud Run but expose FUNCTION_TARGET.
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
// display, read from the cloudservice tags (which already collect it).
func resourceNameFromOrigin(origin string, tags map[string]string) string {
	switch origin {
	case cloudservice.CloudRunOrigin:
		if os.Getenv("FUNCTION_TARGET") != "" {
			return tags["gcrfx.resource_name"]
		}
		return tags["gcr.resource_name"]
	case cloudservice.CloudRunJobsOrigin:
		return tags["gcrj.resource_name"]
	case cloudservice.ContainerAppOrigin:
		return os.Getenv(cloudservice.ContainerAppNameEnvVar)
	case cloudservice.AppServiceOrigin:
		return os.Getenv(cloudservice.WebsiteName)
	}
	return ""
}

// resourceIDFromTags returns the full cloud resource identifier (CCRID) for
// the workload, derived from cloudservice tags. This joins to the crawler's
// canonical_resource_id for the matching resource type. The format MUST match
// the crawler's canonical scheme or the UDM relationship will not resolve.
func resourceIDFromTags(origin string, tags map[string]string) string {
	switch origin {
	case cloudservice.CloudRunOrigin:
		// tags["gcr.resource_name"] = "projects/<p>/locations/<l>/services/<s>"
		// tags["gcrfx.resource_name"] = "projects/<p>/locations/<l>/services/<s>/functions/<f>"
		if os.Getenv("FUNCTION_TARGET") != "" {
			return canonicalGCPRunID(tags["gcrfx.resource_name"])
		}
		return canonicalGCPRunID(tags["gcr.resource_name"])
	case cloudservice.CloudRunJobsOrigin:
		// tags["gcrj.resource_name"] = "projects/<p>/locations/<l>/jobs/<j>"
		return canonicalGCPRunID(tags["gcrj.resource_name"])
	case cloudservice.ContainerAppOrigin:
		// tags["resource_id"] = "/subscriptions/<s>/resourcegroups/<r>/providers/microsoft.app/containerapps/<a>"
		return canonicalAzureID(tags["resource_id"])
	case cloudservice.AppServiceOrigin:
		// AppService tags do not include a resource_id; construct from env.
		sub := os.Getenv(cloudservice.AzureSubscriptionIdEnvVar)
		rg := os.Getenv(cloudservice.AzureResourceGroupEnvVar)
		name := os.Getenv(cloudservice.WebsiteName)
		if sub == "" || rg == "" || name == "" {
			return ""
		}
		return fmt.Sprintf("//microsoft.azure/appServices/%s/%s/%s", sub, rg, strings.ToLower(name))
	}
	return ""
}

// canonicalGCPRunID wraps a Cloud Run "projects/.../services/..." path in the
// run.googleapis.com CCRID scheme used by the crawler.
func canonicalGCPRunID(path string) string {
	if path == "" {
		return ""
	}
	return "//run.googleapis.com/" + path
}

// canonicalAzureID normalizes an Azure ARM resource ID to the lowercase
// //microsoft.azure/... CCRID scheme used by the crawler.
func canonicalAzureID(armID string) string {
	if armID == "" {
		return ""
	}
	// ARM IDs look like "/subscriptions/<s>/resourcegroups/<r>/providers/microsoft.app/containerapps/<a>"
	// Normalize to //microsoft.azure/containerApps/<s>/<r>/<a>
	parts := strings.Split(strings.TrimPrefix(armID, "/"), "/")
	// find subscriptions, resourcegroups, containerapps segments
	var sub, rg, name string
	for i := 0; i < len(parts)-1; i++ {
		switch strings.ToLower(parts[i]) {
		case "subscriptions":
			sub = parts[i+1]
		case "resourcegroups":
			rg = parts[i+1]
		case "containerapps":
			name = parts[i+1]
		}
	}
	if sub == "" || rg == "" || name == "" {
		// fall back to the raw ARM ID lowercased if we can't parse it
		return "//microsoft.azure/" + strings.ToLower(armID)
	}
	return fmt.Sprintf("//microsoft.azure/containerApps/%s/%s/%s", sub, rg, strings.ToLower(name))
}

// gcpProjectIDFromTags extracts the GCP project id from cloudservice tags.
func gcpProjectIDFromTags(tags map[string]string) string {
	if v := tags["project_id"]; v != "" && v != "unknown" {
		return v
	}
	return ""
}

func deploymentModelFromConf(mc mode.Conf) string {
	if mc.SidecarMode {
		return "sidecar"
	}
	return "in-container"
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
