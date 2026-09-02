// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	serverlessInitLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/mode"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CloudRunOrigin origin tag value
const CloudRunOrigin = "cloudrun"

const (
	// Environment var needed for service
	revisionNameEnvVar      = "K_REVISION"
	ServiceNameEnvVar       = "K_SERVICE" // ServiceNameEnvVar is also used in the trace package
	configurationNameEnvVar = "K_CONFIGURATION"
	// exists as cloudrunfunction env var for all runtimes except Go
	functionTypeEnvVar   = "FUNCTION_SIGNATURE_TYPE"
	functionTargetEnvVar = "FUNCTION_TARGET"
)

const (
	// Default values for the metadata service http client requests
	defaultBaseURL        = "http://metadata.google.internal/computeMetadata/v1"
	defaultContainerIDURL = "/instance/id"
	defaultRegionURL      = "/instance/region"
	defaultProjectID      = "/project/project-id"
	defaultTimeout        = 300 * time.Millisecond
)

const (
	// Cloud Run metrics prefixes and names
	cloudRunPrefix             = "gcp.run.container."
	cloudRunShutdownMetricName = "gcp.run.container.enhanced.shutdown"
	cloudRunStartMetricName    = "gcp.run.container.enhanced.cold_start"

	cloudRunLegacyShutdownMetricName = "gcp.run.enhanced.shutdown"
	cloudRunLegacyStartMetricName    = "gcp.run.enhanced.cold_start"

	cloudRunUsageMetricSuffix = "instance"
)

const (
	// Cloud Run common tags
	revisionName = "revision_name"
	serviceName  = "service_name"
	configName   = "configuration_name"
	containerID  = "container_id"
	location     = "location"
	projectID    = "project_id"
	resourceName = "resource_name"
)

const (
	// Cloud Run Service tags
	cloudRunServiceTagPrefix = "gcr."
)

const (
	// Cloud Run Function tags
	cloudRunFunctionTagPrefix = "gcrfx."
	functionTarget            = "build_function_target"
	functionSignature         = "function_signature_type"
)

var metadataHelperFunc = GetMetaData

// GCPConfig holds the metadata configuration
type GCPConfig struct {
	containerIDURL string
	regionURL      string
	projectIDURL   string
	timeout        time.Duration
}

// CloudRun has helper functions for getting Google Cloud Run data. isFunction
// distinguishes a Cloud Run function (gen2) from a plain Cloud Run service and
// is fixed at construction; everything variant-specific derives from it.
type CloudRun struct {
	isFunction bool

	metadataOnce sync.Once
	metadata     map[string]string
}

// resolveMetadata fetches the GCP metadata-service values once and caches them.
// GetTags and GetInventoryData both trigger it, so exactly one network fetch
// happens regardless of call order and neither method depends on the other
// having run first. The cached map is read-only; callers that mutate clone it.
func (c *CloudRun) resolveMetadata() map[string]string {
	c.metadataOnce.Do(func() {
		c.metadata = metadataHelperFunc(GetDefaultConfig(), c.cloudRunType())
	})
	return c.metadata
}

// cloudRunType maps the variant to the metadata enum.
func (c *CloudRun) cloudRunType() CloudRunType {
	if c.isFunction {
		return CloudRunFunction
	}
	return CloudRunService
}

// tagPrefix returns the tag/span namespace prefix for this variant.
func (c *CloudRun) tagPrefix() string {
	if c.isFunction {
		return cloudRunFunctionTagPrefix
	}
	return cloudRunServiceTagPrefix
}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	tags := maps.Clone(c.resolveMetadata())
	tags["origin"] = CloudRunOrigin
	tags["_dd.origin"] = CloudRunOrigin

	prefix := c.tagPrefix()

	if revisionNameVal := os.Getenv(revisionNameEnvVar); revisionNameVal != "" {
		tags[revisionName] = revisionNameVal
		tags[prefix+revisionName] = revisionNameVal
	}

	if serviceNameVal := os.Getenv(ServiceNameEnvVar); serviceNameVal != "" {
		tags[serviceName] = serviceNameVal
		tags[prefix+serviceName] = serviceNameVal
	}

	if configNameVal := os.Getenv(configurationNameEnvVar); configNameVal != "" {
		tags[configName] = configNameVal
		tags[prefix+configName] = configNameVal
	}

	if c.isFunction {
		return c.getFunctionTags(tags)
	}
	tags[cloudRunServiceTagPrefix+resourceName] = cloudRunServiceCCRID(tags[projectID], tags[location], tags[serviceName])
	return tags
}

func (c *CloudRun) GetEnhancedMetricTags(tags map[string]string) EnhancedMetricTags {
	baseTags := map[string]string{
		"location":      tagValueOrUnknown(tags["location"]),
		"origin":        tagValueOrUnknown(tags["origin"]),
		"project_id":    tagValueOrUnknown(tags["project_id"]),
		"revision_name": tagValueOrUnknown(tags["revision_name"]),
		"service_name":  tagValueOrUnknown(tags["service_name"]),
	}

	usageTags := maps.Clone(baseTags)
	usageTags["instance"] = tagValueOrUnknown(tags["container_id"])

	return EnhancedMetricTags{Base: baseTags, Usage: usageTags}
}

func (c *CloudRun) getFunctionTags(tags map[string]string) map[string]string {
	functionTargetVal := os.Getenv(functionTargetEnvVar)
	functionSignatureType := os.Getenv(functionTypeEnvVar)

	if functionTargetVal != "" {
		tags[cloudRunFunctionTagPrefix+functionTarget] = functionTargetVal
	}

	if functionSignatureType != "" {
		tags[cloudRunFunctionTagPrefix+functionSignature] = functionSignatureType
	}

	tags[cloudRunFunctionTagPrefix+resourceName] = cloudRunFunctionCCRID(tags[projectID], tags[location], tags[serviceName], functionTargetVal)
	return tags
}

// cloudRunServiceCCRID builds the service-level Canonical Cloud Resource ID.
// It is the stable parent that revision- and function-level CCRIDs nest under.
func cloudRunServiceCCRID(project, region, service string) string {
	return fmt.Sprintf("projects/%s/locations/%s/services/%s", project, region, service)
}

// cloudRunFunctionCCRID extends the service CCRID with the function segment.
func cloudRunFunctionCCRID(project, region, service, functionTarget string) string {
	return fmt.Sprintf("%s/functions/%s", cloudRunServiceCCRID(project, region, service), functionTarget)
}

// GetInventoryData derives the inventory metadata fields for Cloud Run services
// and functions.
//
// The service-level CCRID is the stable parent. For a service the resource_id
// is the revision under it; parent_resource_id is the service CCRID. For a
// function the resource_id is the function CCRID (which already nests under the
// service path).
func (c *CloudRun) GetInventoryData() InventoryData {
	metadata := c.resolveMetadata()
	project := metadata[projectID]
	region := metadata[location]
	service := os.Getenv(ServiceNameEnvVar)
	revision := os.Getenv(revisionNameEnvVar)

	serviceCCRID := cloudRunServiceCCRID(project, region, service)
	serviceInventoryID := "//run.googleapis.com/" + serviceCCRID

	if c.isFunction {
		return InventoryData{
			WorkloadType:     workloadTypeCloudRunFunction,
			ResourceID:       cloudRunFunctionCCRID(project, region, service, os.Getenv(functionTargetEnvVar)),
			ParentResourceID: serviceInventoryID,
			ResourceName:     service,
			Region:           region,
			GCPProjectID:     project,
			DeploymentID:     revision,
		}
	}

	return InventoryData{
		WorkloadType:     workloadTypeCloudRunService,
		ResourceID:       "//run.googleapis.com/" + cloudRunRevisionCCRID(serviceCCRID, revision),
		ParentResourceID: serviceInventoryID,
		ResourceName:     service,
		Region:           region,
		GCPProjectID:     project,
		DeploymentID:     revision,
	}
}

// cloudRunRevisionCCRID extends a service CCRID with the revision segment. It
// returns the service CCRID unchanged when the revision is unknown so the
// resource_id never dangles on a trailing empty segment.
func cloudRunRevisionCCRID(serviceCCRID, revision string) string {
	if serviceCCRID == "" || revision == "" {
		return serviceCCRID
	}
	return fmt.Sprintf("%s/revisions/%s", serviceCCRID, revision)
}

// GetDefaultLogsSource returns the default logs source if `DD_SOURCE` is not set
func (c *CloudRun) GetDefaultLogsSource() string {
	return CloudRunOrigin
}

func (c *CloudRun) GetMetricPrefix() string {
	return cloudRunPrefix
}

func (c *CloudRun) GetUsageMetricSuffix() string {
	return cloudRunUsageMetricSuffix
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *CloudRun) GetOrigin() string {
	return CloudRunOrigin
}

// GetSource returns the metrics source
func (c *CloudRun) GetSource() metrics.MetricSource {
	return metrics.MetricSourceGoogleCloudRunEnhanced
}

// Init is empty for CloudRun
func (c *CloudRun) Init(_ *TracingContext) error {
	return nil
}

// Run uses the default run behaviour for CloudRun.
func (c *CloudRun) Run(modeConf mode.Conf, logConfig *serverlessInitLog.Config) error {
	return defaultRun(modeConf, logConfig)
}

// Shutdown emits the shutdown metric for CloudRun
func (c *CloudRun) Shutdown(metricAgent *serverlessMetrics.ServerlessMetricAgent, enhancedMetricsEnabled bool, _ error) {
	if metricAgent != nil && enhancedMetricsEnabled {
		metricAgent.AddEnhancedMetric(cloudRunShutdownMetricName, 1.0, c.GetSource(), 0)
		metricAgent.AddLegacyEnhancedMetric(cloudRunLegacyShutdownMetricName, 1.0, c.GetSource())
	}
}

func (c *CloudRun) AddStartMetric(metricAgent *serverlessMetrics.ServerlessMetricAgent) {
	metricAgent.AddEnhancedMetric(cloudRunStartMetricName, 1.0, c.GetSource(), 0)
	metricAgent.AddLegacyEnhancedMetric(cloudRunLegacyStartMetricName, 1.0, c.GetSource())
}

func isCloudRunService() bool {
	_, exists := os.LookupEnv(ServiceNameEnvVar)
	return exists
}

func isCloudRunFunction() bool {
	_, cloudRunFunctionMode := os.LookupEnv(functionTargetEnvVar)
	return cloudRunFunctionMode
}

// GetDefaultConfig returns the medatadata's default config
func GetDefaultConfig() *GCPConfig {
	return &GCPConfig{
		containerIDURL: fmt.Sprintf("%s%s", defaultBaseURL, defaultContainerIDURL),
		regionURL:      fmt.Sprintf("%s%s", defaultBaseURL, defaultRegionURL),
		projectIDURL:   fmt.Sprintf("%s%s", defaultBaseURL, defaultProjectID),
		timeout:        defaultTimeout,
	}
}

func getRegion(httpClient *http.Client, url string) string {
	value := getSingleMetadata(httpClient, url)
	tokens := strings.Split(value, "/")
	return tokens[len(tokens)-1]
}

func getSingleMetadata(httpClient *http.Client, url string) string {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Error("unable to build the metadata request, defaulting to unknown")
		return "unknown"
	}
	req.Header.Add("Metadata-Flavor", "Google")
	res, err := httpClient.Do(req)
	if err != nil {
		log.Info("unable to get the requested metadata, defaulting to unknown")
		return "unknown"
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		log.Error("unable to read metadata body, defaulting to unknown")
		return "unknown"
	}
	return strings.ToLower(string(data))
}

// GetMetaData returns the container's metadata
func GetMetaData(config *GCPConfig, cloudRunType CloudRunType) map[string]string {
	type keyVal struct {
		key, val string
	}
	httpClient := &http.Client{
		Timeout: config.timeout,
	}

	metadata := make(map[string]string, 6)
	metaChan := make(chan keyVal)
	getMeta := func(fnMetadata func(*http.Client, string) string, url string, baseKey string) {
		val := fnMetadata(httpClient, url)
		metaChan <- keyVal{baseKey, val}
		switch cloudRunType {
		case CloudRunJob:
			metaChan <- keyVal{cloudRunJobTagPrefix + baseKey, val}
		case CloudRunService:
			metaChan <- keyVal{cloudRunServiceTagPrefix + baseKey, val}
		case CloudRunFunction:
			metaChan <- keyVal{cloudRunFunctionTagPrefix + baseKey, val}
		default:
			panic(fmt.Sprintf("unexpected cloudRunType for GCP metadata: %s", cloudRunType))
		}
	}

	go getMeta(getSingleMetadata, config.containerIDURL, containerID)
	go getMeta(getRegion, config.regionURL, location)
	go getMeta(getSingleMetadata, config.projectIDURL, projectID)
	timeout := time.After(config.timeout * 6)
	for {
		select {
		case tagSet := <-metaChan:
			metadata[tagSet.key] = tagSet.val
			if len(metadata) == 6 {
				return metadata
			}
		case <-timeout:
			log.Warn("timed out while fetching GCP compute metadata, defaulting to unknown")
			return metadata
		}
	}
}
