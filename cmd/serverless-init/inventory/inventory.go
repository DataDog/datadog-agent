// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package inventory builds the serverless-init inventory payload, flattening
// CloudService platform facts and process-level context into the
// inventoryagent component's metadata fields.
package inventory

import (
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	serverlessTags "github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const reportReasonStartup = "startup"
const reportReasonPeriodic = "periodic"

// serverlessInitFlavor is the flavor reported in the inventory payload only;
// it is not the process-global flavor (flavor.SetFlavor).
const serverlessInitFlavor = "serverless-init"

// NewCapabilities builds initially-not-ready inventoryagent Capabilities for
// serverless-init, reporting one uuid for the process lifetime since serverless
// containers do not share a host GUID.
func NewCapabilities() *inventoryagent.Capabilities {
	id := uuid.New().String()
	return inventoryagent.NewServerlessCapabilities(func() string { return id })
}

// InstanceUUID re-identifies the payload uuid when the process outlives the
// instance it was constructed for.
//
// This type is MicroVM-specific: MicroVM restores many instances from one
// snapshot captured after construction, so every restored instance would
// otherwise report the uuid baked into it. Platforms that run one process per
// deployed instance use NewCapabilities.
type InstanceUUID struct {
	mu         sync.Mutex
	uuid       string
	instanceID string
}

// NewInstanceUUID builds an InstanceUUID.
func NewInstanceUUID() *InstanceUUID {
	return &InstanceUUID{uuid: uuid.New().String()}
}

// SetInstance rotates the uuid when id names an instance other than the current
// one. Same-instance resumes retain the uuid. This relies on the platform
// assumption that snapshot reuse for another instance supplies a new identity;
// it does not detect arbitrary clones. No-op on a nil receiver.
//
// Safe to call concurrently with Resolve.
func (u *InstanceUUID) SetInstance(id string) {
	if u == nil || id == "" {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if id == u.instanceID {
		return
	}
	u.instanceID = id
	u.uuid = uuid.New().String()
}

// Resolve returns the current payload uuid.
func (u *InstanceUUID) Resolve() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.uuid
}

// NewInstanceCapabilities keeps inventory closed during image construction and
// re-resolves the uuid per payload once a lifecycle hook publishes an instance.
func NewInstanceCapabilities(u *InstanceUUID) *inventoryagent.Capabilities {
	return inventoryagent.NewServerlessCapabilities(u.Resolve)
}

// UpdateInstanceAndSubmit updates one MicroVM identity and all matching metadata
// before reopening readiness and immediately submitting. No metadata or UUID
// lock is held while calling SetReady or Submit.
func UpdateInstanceAndSubmit(ia inventoryagent.Component, u *InstanceUUID, id string, cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) {
	if id == "" || !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	ia.SetReady(false)
	u.SetInstance(id)
	Inject(ia, cs, modeConf, conf, tags)
	SetResourceID(ia, conf, id)
	ia.SetReady(true)
	Submit(ia, conf)
}

// UpdateAndSubmit updates all serverless metadata and submits the startup
// payload.
func UpdateAndSubmit(ia inventoryagent.Component, cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	ia.SetReady(false)
	Inject(ia, cs, modeConf, conf, tags)
	ia.SetReady(true)
	Submit(ia, conf)
}

// Inject sets the serverless-specific fields on the inventoryagent component.
// Readiness must be closed while injecting.
func Inject(ia inventoryagent.Component, cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	for key, value := range buildFields(cs, modeConf, conf, tags) {
		ia.Set(key, value)
	}
	ia.Set("flavor", serverlessInitFlavor)
}

// Submit enqueues an inventory payload synchronously.
func Submit(ia inventoryagent.Component, conf configmodel.Reader) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	ia.Submit()
	// The enqueued payload reports startup; switch to periodic for the
	// metadata runner's later submissions.
	ia.Set("report_reason", reportReasonPeriodic)
}

// SetResourceID narrows the resource_id serverless field to the deployed
// instance, for platforms that only learn their instance identifier after the
// initial Inject (e.g. delivered by a lifecycle hook rather than the
// environment).
func SetResourceID(ia inventoryagent.Component, conf configmodel.Reader, id string) {
	if !conf.GetBool("serverless.inventory_enabled") {
		return
	}
	if id == "" {
		log.Debug("serverless-init inventory: no instance id to narrow resource_id with; keeping the id derived from the environment")
		return
	}
	ia.Set("resource_id", id)
}

// buildFields flattens the per-platform inventory data and process-level
// serverless context into the (unprefixed) agent_metadata keys. env, service,
// and version come from the tag map so they match the values this container
// tags its telemetry with; site is not a tag, so it comes from the config.
func buildFields(cs cloudservice.CloudService, modeConf mode.Conf, conf configmodel.Reader, tags map[string]string) map[string]interface{} {
	inv := cs.GetInventoryData()
	var wrappedCommand []string
	if !modeConf.SidecarMode && len(os.Args) > 1 {
		wrappedCommand = os.Args[1:]
	}

	fields := map[string]interface{}{
		"serverless_init_version": serverlessTags.GetExtensionVersion(),
		"agent_version_base":      version.AgentVersion,
		"agent_commit":            version.Commit,
		"report_reason":           reportReasonStartup,

		"resource_id":        inv.ResourceID,
		"resource_name":      inv.ResourceName,
		"workload_type":      inv.WorkloadType,
		"parent_resource_id": inv.ParentResourceID,

		"region":                inv.Region,
		"gcp_project_id":        inv.GCPProjectID,
		"aws_account_id":        inv.AWSAccountID,
		"azure_subscription_id": inv.AzureSubscriptionID,
		"azure_resource_group":  inv.AzureResourceGroup,

		"deployment_model": deploymentModel(modeConf),
		"runtime":          resolveRuntime(inv.RuntimeCandidates, wrappedCommand),

		"dd_env":     tags["env"],
		"dd_site":    conf.GetString("site"),
		"dd_version": tags["version"],
		"dd_service": tags["service"],
	}

	// wrapped_command is opt-in because redaction is best effort; it is set to
	// nil when disabled to clear any value cached by the component.
	fields["wrapped_command"] = nil
	if conf.GetBool("serverless.inventory_wrapped_command_enabled") && len(wrappedCommand) > 0 {
		fields["wrapped_command"] = scrubber.ScrubLine(strings.Join(wrappedCommand, " "))
	}

	for key, value := range fields {
		if text, ok := value.(string); ok && text == "" {
			// Passing nil also clears a previously populated value in the component's cache.
			fields[key] = nil
		}
	}

	return fields
}

func deploymentModel(modeConf mode.Conf) string {
	if modeConf.SidecarMode {
		return "sidecar"
	}
	return "in-container"
}
