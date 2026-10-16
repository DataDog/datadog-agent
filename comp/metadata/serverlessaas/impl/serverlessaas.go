// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverlessaas implements Azure App Service extension inventory.
package serverlessaasimpl

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorypayload"
	runnerdef "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	serverlessaasdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessaas/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

const (
	aasInventoryFlavor = "serverless-extension"

	reportReasonStartup  = "startup"
	reportReasonPeriodic = "periodic"

	workloadTypeAzureAppService = "azure_app_service"
	workloadTypeAzureFunction   = "azure_function"

	// envInventoryEnabled gates AAS inventory reporting. Set it to "1" in the
	// Function App / Web App app settings to enable inventory.
	envInventoryEnabled = "DD_SERVERLESS_AAS_EXTENSION_INVENTORY_ENABLED"
)

type serverlessAASComp struct {
	payload   util.InventoryPayload
	data      *inventorypayload.Data
	conf      config.Component
	log       log.Component
	hostname  hostnameinterface.Component
	uuid      string
	submitted bool
}

// Requires defines the dependencies for the AAS inventory component.
type Requires struct {
	compdef.In

	Config     config.Component
	Hostname   hostnameinterface.Component
	Log        log.Component
	Serializer serializer.MetricSerializer
}

// Provides defines the output of the AAS inventory component.
type Provides struct {
	compdef.Out

	Comp     serverlessaasdef.Component
	Provider runnerdef.Provider
}

// NewComponent registers only the AAS inventory provider. The normal metadata
// runner owns startup, periodic collection, and cancellation.
func NewComponent(deps Requires) Provides {
	c := &serverlessAASComp{
		conf:     deps.Config,
		log:      deps.Log,
		hostname: deps.Hostname,
		uuid:     uuid.New().String(),
	}
	var metadataSerializer serializer.MetricSerializer
	if deps.Serializer != nil {
		metadataSerializer = submissionRecordingSerializer{
			MetricSerializer: deps.Serializer,
			onSubmitted:      c.recordSubmission,
		}
	}
	c.payload = util.CreateInventoryPayload(immediateInventoryConfig{deps.Config}, deps.Log, metadataSerializer, c.getPayload, "agent.json")
	c.payload.Enabled = c.payload.Enabled && isEnabled()

	return Provides{
		Comp:     c,
		Provider: c.payload.MetadataProvider(),
	}
}

// immediateInventoryConfig skips the host startup delay only for this provider.
// The shared configuration and other metadata providers retain their settings.
type immediateInventoryConfig struct{ config.Component }

func (c immediateInventoryConfig) GetDuration(key string) time.Duration {
	if key == "inventories_first_run_delay" {
		return 0
	}
	return c.Component.GetDuration(key)
}

func isEnabled() bool {
	return os.Getenv(envInventoryEnabled) == "1"
}

func (c *serverlessAASComp) getPayload() marshaler.JSONMarshaler {
	fields := buildFields(c.conf)
	if fields == nil {
		return nil
	}
	if c.submitted {
		fields["report_reason"] = reportReasonPeriodic
	}
	if c.data == nil {
		c.data = inventorypayload.NewData(c.conf, c.log, c.hostname)
	}
	return c.data.GetPayload(fields, c.uuid)
}

func (c *serverlessAASComp) recordSubmission() {
	c.submitted = true
}

type submissionRecordingSerializer struct {
	serializer.MetricSerializer
	onSubmitted func()
}

func (s submissionRecordingSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	if err := s.MetricSerializer.SendMetadata(payload); err != nil {
		return err
	}
	s.onSubmitted()
	return nil
}

// buildFields snapshots the complete AAS field set. A nil result withholds the
// payload when the required REDAPL resource key cannot be derived.
func buildFields(conf config.Component) map[string]interface{} {
	metadata := readAppServiceMetadata()
	resourceID := canonicalResourceID(metadata.resourceID)
	if resourceID == "" {
		return nil
	}

	return map[string]interface{}{
		"flavor":                aasInventoryFlavor,
		"report_reason":         reportReasonStartup,
		"resource_id":           resourceID,
		"resource_name":         metadata.siteName,
		"workload_type":         workloadType(),
		"region":                os.Getenv("REGION_NAME"),
		"azure_subscription_id": metadata.subscriptionID,
		"azure_resource_group":  metadata.resourceGroup,
		"runtime":               metadata.runtime,
		"extension_version":     metadata.extensionVersion,
		"dd_env":                conf.GetString("env"),
		"dd_site":               conf.GetString("site"),
		"dd_service":            os.Getenv("DD_SERVICE"),
		"dd_version":            os.Getenv("DD_VERSION"),
	}
}

func workloadType() string {
	if strings.TrimSpace(os.Getenv("WEBSITE_SITE_NAME")) == "" {
		return ""
	}
	if _, isFunctionApp := os.LookupEnv("FUNCTIONS_WORKER_RUNTIME"); isFunctionApp {
		return workloadTypeAzureFunction
	}
	return workloadTypeAzureAppService
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
