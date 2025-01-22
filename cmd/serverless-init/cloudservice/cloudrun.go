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
	"sync"
	"time"
)

const (
	// Environment var needed for service
	revisionNameEnvVar      = "K_REVISION"
	ServiceNameEnvVar       = "K_SERVICE" // ServiceNameEnvVar is also used in the trace package
	configurationNameEnvVar = "K_CONFIGURATION"
	functionTypeEnvVar      = "FUNCTION_SIGNATURE_TYPE"
	functionTargetEnvVar    = "FUNCTION_TARGET" // exists as a cloudrunfunction env var for all runtimes except Go
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
	cloudRunService      = "gcr."
	cloudRunFunction     = "gcrfx."
	runRevisionName      = "gcr.revision_name"
	functionRevisionName = "gcrfx.revision_name"
	runServiceName       = "gcr.service_name"
	functionServiceName  = "gcrfx.service_name"
	runConfigName        = "gcr.configuration_name"
	functionConfigName   = "gcrfx.configuration_name"
	runContainerID       = "gcr.container_id"
	functionContainerID  = "gcrfx.container_id"
	runLocation          = "gcr.location"
	functionLocation     = "gcrfx.location"
	runProjectID         = "gcr.project_id"
	functionProjectID    = "gcrfx.project_id"
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

	revisionName := os.Getenv(revisionNameEnvVar)
	serviceName := os.Getenv(ServiceNameEnvVar)
	configName := os.Getenv(configurationNameEnvVar)
	if revisionName != "" {
		tags["revision_name"] = revisionName
		if isCloudRun {
			tags[runRevisionName] = revisionName
		} else {
			tags[functionRevisionName] = revisionName
		}
	}

	if serviceName != "" {
		tags["service_name"] = serviceName
		if isCloudRun {
			tags[runServiceName] = serviceName
		} else {
			tags[functionServiceName] = serviceName
		}
	}

	if configName != "" {
		tags["configuration_name"] = configName
		if isCloudRun {
			tags[runConfigName] = configName
		} else {
			tags[functionConfigName] = configName
		}
	}

	if c.spanNamespace == cloudRunFunction {
		return c.getFunctionTags(tags)
	}

	tags["gcr.resource_name"] = "projects/" + tags["project_id"] + "/locations/" + tags["location"] + "/services/" + serviceName
	return tags
}

func (c *CloudRun) getFunctionTags(tags map[string]string) map[string]string {
	functionTarget := os.Getenv(functionTargetEnvVar)
	functionSignatureType := os.Getenv(functionTypeEnvVar)

	if functionTarget != "" {
		tags[c.spanNamespace+"function_target"] = functionTarget
	}

	if functionSignatureType != "" {
		tags[c.spanNamespace+"function_signature_type"] = functionSignatureType
	}

	tags["gcrfx.resource_name"] = "projects/" + tags["project_id"] + "/locations/" + tags["location"] + "/services/" + tags["service_name"] + "/functions/" + functionTarget
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

func getRegion(httpClient *http.Client, config *GCPConfig) string {
	value := getSingleMetadata(httpClient, config.regionURL)
	tokens := strings.Split(value, "/")
	return tokens[len(tokens)-1]
}

// GetMetaData returns the container's metadata
func GetMetaData(config *GCPConfig, isCloudRun bool) map[string]string {
	wg := sync.WaitGroup{}
	wg.Add(3)
	httpClient := &http.Client{
		Timeout: config.timeout,
	}
	metadata := make(map[string]string)
	go func() {
		containerID := getSingleMetadata(httpClient, config.containerIDURL)
		metadata["container_id"] = containerID
		if isCloudRun {
			metadata[runContainerID] = containerID
		} else {
			metadata[functionContainerID] = containerID
		}
		wg.Done()
	}()
	go func() {
		location := getRegion(httpClient, config)
		metadata["location"] = location
		if isCloudRun {
			metadata[runLocation] = location
		} else {
			metadata[functionLocation] = location
		}
		wg.Done()
	}()
	go func() {
		project := getSingleMetadata(httpClient, config.projectIDURL)
		metadata["project_id"] = project
		if isCloudRun {
			metadata[runProjectID] = project
		} else {
			metadata[functionProjectID] = project
		}
		wg.Done()
	}()
	wg.Wait()
	return metadata
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
