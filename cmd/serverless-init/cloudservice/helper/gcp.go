// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package helper

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultBaseURL = "http://metadata.google.internal/computeMetadata/v1"
const defaultContainerIDURL = "/instance/id"
const defaultRegionURL = "/instance/region"
const defaultProjectID = "/project/project-id"
const defaultTimeout = 300 * time.Millisecond

// GCPConfig holds the metadata configuration
type GCPConfig struct {
	containerIDURL string
	regionURL      string
	projectIDURL   string
	timeout        time.Duration
}

// Info holds the GCP tag value format
type Info struct {
	TagName string
	Value   string
}

// GCPMetadata holds the container's metadata
type GCPMetadata struct {
	ContainerID *Info
	Region      *Info
	ProjectID   *Info
}

// TagMap returns the container's metadata in a map
func (metadata *GCPMetadata) TagMap() map[string]string {
	tagMap := map[string]string{}
	if metadata.ContainerID != nil {
		tagMap[metadata.ContainerID.TagName] = metadata.ContainerID.Value
	}
	if metadata.Region != nil {
		tagMap[metadata.Region.TagName] = metadata.Region.Value
	}
	if metadata.ProjectID != nil {
		tagMap[metadata.ProjectID.TagName] = metadata.ProjectID.Value
	}
	return tagMap
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

// GetMetaData returns the container's metadata
func GetMetaData(config *GCPConfig) *GCPMetadata {
	wg := sync.WaitGroup{}
	wg.Add(3)
	httpClient := &http.Client{
		Timeout: config.timeout,
	}
	metadata := &GCPMetadata{}
	go func() {
		metadata.ContainerID = getContainerID(httpClient, config)
		wg.Done()
	}()
	go func() {
		metadata.Region = getRegion(httpClient, config)
		wg.Done()
	}()
	go func() {
		metadata.ProjectID = getProjectID(httpClient, config)
		wg.Done()
	}()
	wg.Wait()
	return metadata
}

func getContainerID(httpClient *http.Client, config *GCPConfig) *Info {
	return &Info{
		TagName: "container_id",
		Value:   getSingleMetadata(httpClient, config.containerIDURL),
	}
}

func getRegion(httpClient *http.Client, config *GCPConfig) *Info {
	value := getSingleMetadata(httpClient, config.regionURL)
	tokens := strings.Split(value, "/")
	return &Info{
		TagName: "location",
		Value:   tokens[len(tokens)-1],
	}
}

func getProjectID(httpClient *http.Client, config *GCPConfig) *Info {
	return &Info{
		TagName: "project_id",
		Value:   getSingleMetadata(httpClient, config.projectIDURL),
	}
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
