// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package aas wires the Azure App Service (.NET extension) dogstatsd.exe
// process to the shared inventoryagent component so it can emit a serverless
// inventory payload. All AAS-specific metadata derivation lives here; the
// shared component stays generic.
package aas

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const aasInventoryFlavor = "serverless-extension"
const defaultInventoryReportInterval = 10 * time.Minute

const (
	reportReasonStartup  = "startup"
	reportReasonPeriodic = "periodic"

	workloadTypeAzureAppService = "azure_app_service"
	workloadTypeAzureFunction   = "azure_function"

	// envInventoryEnabled gates AAS inventory reporting. Set it to "1" in the
	// Function App / Web App app settings to enable inventory.
	envInventoryEnabled = "DD_SERVERLESS_AAS_EXTENSION_INVENTORY_ENABLED"
)

// IsEnabled reports whether AAS inventory reporting is active.
func IsEnabled() bool {
	return os.Getenv(envInventoryEnabled) == "1"
}

// NewCapabilities returns the inventoryagent Capabilities for dogstatsd running
// inside the AAS extension: skip cross-process enrichment (no sibling agent
// processes), use a per-process UUID, and enable only this inventory payload.
// The AAS extension's scoped gate does not enable unrelated metadata providers.
func NewCapabilities() *inventoryagent.Capabilities {
	id := uuid.New().String()
	caps := inventoryagent.NewServerlessCapabilities(func() string { return id })
	caps.EnableInventoryPayload = true
	return caps
}

// workloadType returns the downstream workload_type value for this AAS process.
// Azure Function Apps set FUNCTIONS_WORKER_RUNTIME; plain Web Apps do not.
func workloadType() string {
	if strings.TrimSpace(os.Getenv("WEBSITE_SITE_NAME")) == "" {
		return ""
	}
	if strings.TrimSpace(os.Getenv("FUNCTIONS_WORKER_RUNTIME")) != "" {
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

	aasMetadata := readAppServiceMetadata()
	// Azure resource IDs are case-insensitive, while the casing returned by
	// Azure APIs is inconsistent. REDAPL canonicalizes Azure keys to lowercase.
	resourceID := canonicalResourceID(aasMetadata.resourceID)
	if resourceID == "" {
		// Cannot form a valid REDAPL key; skip rather than emit a dangling row.
		return false
	}

	ia.Set("flavor", aasInventoryFlavor)
	ia.Set("report_reason", reportReasonStartup)

	ia.Set("resource_id", resourceID)
	ia.Set("resource_name", aasMetadata.siteName)
	ia.Set("workload_type", workloadType())

	ia.Set("region", os.Getenv("REGION_NAME"))
	ia.Set("azure_subscription_id", aasMetadata.subscriptionID)
	ia.Set("azure_resource_group", aasMetadata.resourceGroup)
	ia.Set("runtime", aasMetadata.runtime)
	ia.Set("extension_version", aasMetadata.extensionVersion)

	ia.Set("dd_env", conf.GetString("env"))
	ia.Set("dd_site", conf.GetString("site"))
	ia.Set("dd_service", os.Getenv("DD_SERVICE"))
	ia.Set("dd_version", os.Getenv("DD_VERSION"))
	return true
}

type appServiceMetadata struct {
	resourceID       string
	siteName         string
	subscriptionID   string
	resourceGroup    string
	runtime          string
	extensionVersion string
}

// readAppServiceMetadata reads only the AAS values needed by this payload. This
// keeps dogstatsd independent of traceutil and its trace protobuf dependencies.
func readAppServiceMetadata() appServiceMetadata {
	siteName := os.Getenv("WEBSITE_SITE_NAME")
	resourceGroup := os.Getenv("WEBSITE_RESOURCE_GROUP")
	ownerName := os.Getenv("WEBSITE_OWNER_NAME")
	subscriptionID := ""
	if parts := strings.SplitN(ownerName, "+", 2); len(parts) == 2 {
		subscriptionID = parts[0]
	}

	resourceID := ""
	if subscriptionID != "" && resourceGroup != "" && siteName != "" {
		resourceID = fmt.Sprintf(
			"/subscriptions/%s/resourcegroups/%s/providers/microsoft.web/sites/%s",
			subscriptionID,
			resourceGroup,
			siteName,
		)
	}

	return appServiceMetadata{
		resourceID:       resourceID,
		siteName:         siteName,
		subscriptionID:   subscriptionID,
		resourceGroup:    resourceGroup,
		runtime:          appServiceRuntime(),
		extensionVersion: firstNonEmptyEnv("DD_AAS_EXTENSION_VERSION", "DD_AAS_JAVA_EXTENSION_VERSION", "DD_AAS_DOTNET_EXTENSION_VERSION"),
	}
}

func appServiceRuntime() string {
	if runtime := strings.TrimSpace(os.Getenv("FUNCTIONS_WORKER_RUNTIME")); runtime != "" {
		return runtime
	}
	if os.Getenv("WEBSITE_STACK") == "JAVA" {
		return "Java"
	}
	if os.Getenv("WEBSITE_NODE_DEFAULT_VERSION") != "" {
		return "Node.js"
	}
	// The Windows AAS extension supports .NET, Java, and Node.js. If neither
	// Java nor Node.js is selected, the App Service runtime is .NET.
	return ".NET"
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}

func canonicalResourceID(baseResourceID string) string {
	resourceID := strings.ToLower(baseResourceID)
	if resourceID == "" {
		return ""
	}

	slot := strings.TrimSpace(os.Getenv("WEBSITE_SLOT_NAME"))
	if slot != "" && !strings.EqualFold(slot, "production") {
		resourceID += "/slots/" + strings.ToLower(slot)
	}
	return resourceID
}

// Submit builds and enqueues the inventory payload synchronously before the
// metadata runner goroutine fires. It is a no-op when IsEnabled() is false.
func Submit(ia inventoryagent.Component) {
	if !IsEnabled() {
		return
	}
	ia.Submit()
	ia.Set("report_reason", reportReasonPeriodic)
}

// Module returns the fx.Option that wires AAS inventory for dogstatsd running
// inside an Azure App Service extension. It provides the Capabilities and
// registers an OnStart hook that injects fields and enqueues the initial payload.
func Module() fx.Option {
	if !IsEnabled() {
		return fx.Options()
	}
	return fx.Options(
		fx.Provide(NewCapabilities),
		fx.Invoke(setupLifecycle),
	)
}

type lifecycleDeps struct {
	fx.In
	Lc             fx.Lifecycle
	InventoryAgent inventoryagent.Component
	Config         coreconfig.Component
}

func setupLifecycle(deps lifecycleDeps) {
	if !IsEnabled() {
		return
	}
	var cancelPeriodic context.CancelFunc
	var periodicStopped <-chan struct{}
	deps.Lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if Inject(deps.InventoryAgent, deps.Config) {
				Submit(deps.InventoryAgent)
				// When the normal metadata runner is disabled, keep only the AAS
				// inventory payload periodic instead of starting every registered
				// dogstatsd metadata provider.
				if !deps.Config.GetBool("enable_metadata_collection") {
					periodicCtx, cancel := context.WithCancel(context.Background())
					stopped := make(chan struct{})
					cancelPeriodic = cancel
					periodicStopped = stopped
					go func() {
						defer close(stopped)
						runPeriodic(
							periodicCtx,
							deps.InventoryAgent,
							inventoryReportInterval(deps.Config),
						)
					}()
				}
			}
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if cancelPeriodic == nil {
				return nil
			}
			cancelPeriodic()
			select {
			case <-periodicStopped:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}

func inventoryReportInterval(conf configmodel.Reader) time.Duration {
	configuredSeconds := conf.GetInt("inventories_max_interval")
	interval := time.Duration(configuredSeconds) * time.Second
	if interval <= 0 {
		log.Debugf(
			"AAS inventory: inventories_max_interval=%d is not positive; using default interval %s",
			configuredSeconds,
			defaultInventoryReportInterval,
		)
		return defaultInventoryReportInterval
	}
	return interval
}

func runPeriodic(ctx context.Context, ia inventoryagent.Component, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	submitOnTicks(ctx, ia, ticker.C)
}

func submitOnTicks(ctx context.Context, ia inventoryagent.Component, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
			Submit(ia)
		}
	}
}
