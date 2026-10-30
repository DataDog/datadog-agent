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
	"sync/atomic"
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

// gcpMetadataCache holds the GCP metadata-service values fetched by the first
// GetTags call.
type gcpMetadataCache struct {
	once     sync.Once
	resolved atomic.Pointer[map[string]string]
}

// resolve fetches the metadata-service values once. Failed lookups are cached
// as empty values. The returned map is shared and must not be mutated.
func (g *gcpMetadataCache) resolve(cloudRunType CloudRunType) map[string]string {
	g.once.Do(func() {
		metadata := metadataHelperFunc(GetDefaultConfig(), cloudRunType)
		g.resolved.Store(&metadata)
	})
	return *g.resolved.Load()
}

// tags returns a copy of the resolved values without the failed lookups, so
// telemetry carries no empty-valued tags.
func (g *gcpMetadataCache) tags(cloudRunType CloudRunType) map[string]string {
	tags := maps.Clone(g.resolve(cloudRunType))
	maps.DeleteFunc(tags, func(_, value string) bool { return value == "" })
	return tags
}

// CloudRun has helper functions for getting Google Cloud Run data. isFunction
// distinguishes a Gen2 Cloud Run function from a Cloud Run service.
type CloudRun struct {
	isFunction  bool
	gcpMetadata gcpMetadataCache
}

func (c *CloudRun) cloudRunType() CloudRunType {
	if c.isFunction {
		return CloudRunFunction
	}
	return CloudRunService
}

func (c *CloudRun) tagPrefix() string {
	if c.isFunction {
		return cloudRunFunctionTagPrefix
	}
	return cloudRunServiceTagPrefix
}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	tags := c.gcpMetadata.tags(c.cloudRunType())
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
	if id := cloudRunServiceCCRID(tags[projectID], tags[location], tags[serviceName]); id != "" {
		tags[cloudRunServiceTagPrefix+resourceName] = id
	}
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

	if id := cloudRunFunctionCCRID(tags[projectID], tags[location], tags[serviceName], functionTargetVal); id != "" {
		tags[cloudRunFunctionTagPrefix+resourceName] = id
	}
	return tags
}

// cloudRunServiceCCRID builds the service-level Canonical Cloud Resource ID.
func cloudRunServiceCCRID(project, region, service string) string {
	if project == "" || region == "" || service == "" {
		return ""
	}
	return fmt.Sprintf("projects/%s/locations/%s/services/%s", project, region, service)
}

func cloudRunFunctionCCRID(project, region, service, functionTarget string) string {
	parent := cloudRunServiceCCRID(project, region, service)
	if parent == "" || functionTarget == "" {
		return ""
	}
	return fmt.Sprintf("%s/functions/%s", parent, functionTarget)
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
		log.Error("unable to build the metadata request")
		return ""
	}
	req.Header.Add("Metadata-Flavor", "Google")
	res, err := httpClient.Do(req)
	if err != nil {
		log.Info("unable to get the requested metadata")
		return ""
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Infof("metadata request failed with HTTP status %d", res.StatusCode)
		return ""
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		log.Error("unable to read metadata body")
		return ""
	}
	return strings.ToLower(string(data))
}

// GetMetaData returns the container's metadata. Failed lookups have empty values.
func GetMetaData(config *GCPConfig, cloudRunType CloudRunType) map[string]string {
	type keyVal struct {
		key, val string
	}
	httpClient := &http.Client{
		Timeout: config.timeout,
	}

	metadata := make(map[string]string, 6)
	// Buffer all results so late requests can finish after the outer timeout.
	metaChan := make(chan keyVal, 6)
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
			log.Warn("timed out while fetching GCP compute metadata")
			return metadata
		}
	}
}
