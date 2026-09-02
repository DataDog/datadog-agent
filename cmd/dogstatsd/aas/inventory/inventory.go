// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventory wires the Azure App Service (.NET extension) dogstatsd.exe
// process to the shared inventoryagent component so it can emit a serverless
// inventory payload. All AAS-specific metadata derivation lives here; the
// shared component stays generic.
//
// Temporary: the payload currently uses flavor "serverless-compat" so rows
// land in the existing serverless_compat_agent REDAPL table for the sanity
// test. Once serverless_extension_agent is created in dd-source and EPRW,
// change aasInventoryFlavor to "serverless-extension".
package inventory

import (
	"os"

	"github.com/google/uuid"

	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// TODO(SVLS-9604): change to "serverless-extension" once serverless_extension_agent schema exists.
const aasInventoryFlavor = "serverless-compat"

const (
	reportReasonStartup  = "startup"
	reportReasonPeriodic = "periodic"

	workloadTypeAzureAppService = "azure_app_service"
	workloadTypeAzureFunction   = "azure_function"

	// envInventoryEnabled gates AAS inventory reporting. Set to "1" in the
	// Function App / Web App app settings to enable. Independent of the
	// serverless-init gate so each producer rolls out separately.
	envInventoryEnabled = "DD_SERVERLESS_AAS_EXTENSION_INVENTORY_ENABLED"
)

// IsEnabled reports whether AAS inventory reporting is active.
func IsEnabled() bool {
	return os.Getenv(envInventoryEnabled) == "1"
}

// NewCapabilities returns the inventoryagent Capabilities for dogstatsd running
// inside the AAS extension: skip cross-process enrichment (no sibling agent
// processes) and use a per-process UUID. Multiple workers sharing the same app
// produce separate startup payloads but the REDAPL row deduplicates on
// resource_id, so cardinality stays at one row per app.
func NewCapabilities() *inventoryagent.Capabilities {
	return inventoryagent.NewServerlessCapabilities(uuid.New().String())
}

// workloadType returns the downstream workload_type value for this AAS process.
// Azure Function Apps set FUNCTIONS_WORKER_RUNTIME; plain Web Apps do not.
func workloadType() string {
	if _, ok := os.LookupEnv("FUNCTIONS_WORKER_RUNTIME"); ok {
		return workloadTypeAzureFunction
	}
	return workloadTypeAzureAppService
}

// Inject sets the AAS-specific inventory fields on the shared inventoryagent
// component. It is a no-op when IsEnabled() is false or when the Azure
// resource ID cannot be derived (required REDAPL key; prevents a dangling row).
//
// Fields use unprefixed names (resource_id, workload_type, …) as required by
// the EPRW decoder. agent_version_base and extension_version are omitted for
// the temporary serverless-compat sanity test; add them when switching to
// serverless-extension.
func Inject(ia inventoryagent.Component, conf configmodel.Reader) {
	if !IsEnabled() {
		return
	}

	aasTags := traceutil.GetAppServicesTags()
	resourceID := aasTags[traceutil.AASResourceID]
	if resourceID == "" {
		// Cannot form a valid REDAPL key; skip rather than emit a dangling row.
		return
	}

	ia.Set("flavor", aasInventoryFlavor)
	ia.Set("report_reason", reportReasonStartup)

	ia.Set("resource_id", resourceID)
	ia.Set("resource_name", os.Getenv("WEBSITE_SITE_NAME"))
	ia.Set("workload_type", workloadType())

	ia.Set("region", os.Getenv("REGION_NAME"))
	ia.Set("azure_subscription_id", aasTags[traceutil.AASSubscriptionID])
	ia.Set("azure_resource_group", aasTags[traceutil.AASResourceGroup])
	ia.Set("runtime", aasTags[traceutil.AASRuntime])

	ia.Set("dd_env", conf.GetString("env"))
	ia.Set("dd_site", conf.GetString("site"))
	ia.Set("dd_service", os.Getenv("DD_SERVICE"))
	ia.Set("dd_version", os.Getenv("DD_VERSION"))
}

// Submit enqueues the inventory payload synchronously so it is delivered before
// the metadata runner goroutine fires. After submission it switches
// report_reason to "periodic" so subsequent runner cycles (~10 min) are tagged
// correctly without another Inject call.
//
// It is a no-op when IsEnabled() is false.
func Submit(ia inventoryagent.Component) {
	if !IsEnabled() {
		return
	}
	ia.Submit()
	ia.Set("report_reason", reportReasonPeriodic)
}
