// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

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
	// Span Tag with namespace specific for cloud run (gcr) and cloud run function (gcrfx)
	cloudRunService   = "gcr."
	cloudRunFunction  = "gcrfx."
	revisionName      = "revision_name"
	serviceName       = "service_name"
	configName        = "configuration_name"
	containerID       = "container_id"
	location          = "location"
	projectID         = "project_id"
	resourceName      = "resource_name"
	functionTarget    = "function_target"
	functionSignature = "function_signature_type"
)

var metadataHelperFunc = GetMetaData

// GCPConfig holds the metadata configuration
type GCPConfig struct {
	containerIDURL string
	regionURL      string
	projectIDURL   string
	timeout        time.Duration
}

// CloudRun has helper functions for getting Google Cloud Run data
type CloudRun struct {
	spanNamespace string
}

// GetTags returns a map of gcp-related tags.
func (c *CloudRun) GetTags() map[string]string {
	isCloudRun := c.spanNamespace == cloudRunService
	tags := metadataHelperFunc(GetDefaultConfig(), isCloudRun)
	tags["origin"] = c.GetOrigin()
	tags["_dd.origin"] = c.GetOrigin()

	revisionNameVal := os.Getenv(revisionNameEnvVar)
	serviceNameVal := os.Getenv(ServiceNameEnvVar)
	configNameVal := os.Getenv(configurationNameEnvVar)
	if revisionNameVal != "" {
		tags[revisionName] = revisionNameVal
		if isCloudRun {
			tags[cloudRunService+revisionName] = revisionNameVal
		} else {
			tags[cloudRunFunction+revisionName] = revisionNameVal
		}
	}

	if serviceNameVal != "" {
		tags[serviceName] = serviceNameVal
		if isCloudRun {
			tags[cloudRunService+serviceName] = serviceNameVal
		} else {
			tags[cloudRunFunction+serviceName] = serviceNameVal
		}
	}

	if configNameVal != "" {
		tags[configName] = configNameVal
		if isCloudRun {
			tags[cloudRunService+configName] = configNameVal
		} else {
			tags[cloudRunFunction+configName] = configNameVal
		}
	}

	if c.spanNamespace == cloudRunFunction {
		return c.getFunctionTags(tags)
	}
	tags[cloudRunService+resourceName] = fmt.Sprintf("projects/%s/locations/%s/services/%s", tags["project_id"], tags["location"], tags["service_name"])
	return tags
}

func (c *CloudRun) getFunctionTags(tags map[string]string) map[string]string {
	functionTargetVal := os.Getenv(functionTargetEnvVar)
	functionSignatureType := os.Getenv(functionTypeEnvVar)

	if functionTargetVal != "" {
		tags[cloudRunFunction+functionTarget] = functionTargetVal
	}

	if functionSignatureType != "" {
		tags[cloudRunFunction+functionSignature] = functionSignatureType
	}

	tags[cloudRunFunction+resourceName] = fmt.Sprintf("projects/%s/locations/%s/services/%s/functions/%s", tags["project_id"], tags["location"], tags["service_name"], functionTargetVal)
	return tags
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *CloudRun) GetOrigin() string {
	return "cloudrun"
}

// GetPrefix returns the prefix that we're prefixing all
// metrics with.
func (c *CloudRun) GetPrefix() string {
	return "gcp.run"
}

// Init is empty for CloudRun
func (c *CloudRun) Init() error {
	return nil
}

func isCloudRunService() bool {
	_, exists := os.LookupEnv(ServiceNameEnvVar)
	return exists
}

func isCloudRunFunction() bool {
	_, cloudRunFunctionMode := os.LookupEnv(functionTargetEnvVar)
	log.Debug(fmt.Sprintf("cloud run namespace SET TO: %s", cloudRunFunction))
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
func GetMetaData(config *GCPConfig, isCloudRun bool) map[string]string {
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
		if isCloudRun {
			metaChan <- keyVal{cloudRunService + baseKey, val}
		} else {
			metaChan <- keyVal{cloudRunFunction + baseKey, val}
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
