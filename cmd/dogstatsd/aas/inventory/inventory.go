// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventory wires the Azure App Service (.NET extension) dogstatsd.exe
// process to the shared inventoryagent component so it can emit a serverless
// inventory payload. All AAS-specific metadata derivation lives here; the
// shared component stays generic.
package inventory

import (
	"os"

	"github.com/google/uuid"

	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const aasInventoryFlavor = "serverless-extension"

const (
	reportReasonStartup = "startup"

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
// processes), use a per-process UUID, and force the payload enabled so AAS
// inventory works regardless of the enable_metadata_collection config flag.
func NewCapabilities() *inventoryagent.Capabilities {
	caps := inventoryagent.NewServerlessCapabilities(uuid.New().String())
	caps.ForceEnabled = true
	return caps
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
// component. It returns true when fields were set and Submit should be called.
// It returns false when IsEnabled() is false or when the Azure resource ID
// cannot be derived (required REDAPL key; prevents a dangling row).
//
// Fields use unprefixed names (resource_id, workload_type, …) as required by
// the EPRW decoder.
func Inject(ia inventoryagent.Component, conf configmodel.Reader) bool {
	if !IsEnabled() {
		return false
	}

	aasTags := traceutil.GetAppServicesTags()
	resourceID := aasTags[traceutil.AASResourceID]
	if resourceID == "" {
		// Cannot form a valid REDAPL key; skip rather than emit a dangling row.
		return false
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
	ia.Set("extension_version", aasTags[traceutil.AASExtensionVersion])

	ia.Set("dd_env", conf.GetString("env"))
	ia.Set("dd_site", conf.GetString("site"))
	ia.Set("dd_service", os.Getenv("DD_SERVICE"))
	ia.Set("dd_version", os.Getenv("DD_VERSION"))
	return true
}

// Submit enqueues the inventory payload synchronously so it is delivered before
// the metadata runner goroutine fires. It is a no-op when IsEnabled() is false.
// Subsequent periodic submissions are handled by the inventoryagent built-in
// runner (defaultMaxInterval = 10 min), so no separate goroutine is needed.
func Submit(ia inventoryagent.Component) {
	if !IsEnabled() {
		return
	}
	ia.Submit()
}
