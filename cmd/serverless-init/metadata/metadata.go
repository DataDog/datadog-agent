// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"fmt"
	"io/ioutil"
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

// Config holds the metadata configuration
type Config struct {
	containerIDURL string
	regionURL      string
	projectIDURL   string
	timeout        time.Duration
}

type info struct {
	tagName string
	value   string
}

// Metadata holds the container's metadata
type Metadata struct {
	containerID *info
	region      *info
	projectID   *info
}

// TagMap returns the container's metadata in a map
func (metadata *Metadata) TagMap() map[string]string {
	tagMap := map[string]string{}
	if metadata.containerID != nil {
		tagMap[metadata.containerID.tagName] = metadata.containerID.value
	}
	if metadata.region != nil {
		tagMap[metadata.region.tagName] = metadata.region.value
	}
	if metadata.projectID != nil {
		tagMap[metadata.projectID.tagName] = metadata.projectID.value
	}
	return tagMap
}

// GetDefaultConfig returns the medatadata's default config
func GetDefaultConfig() *Config {
	return &Config{
		containerIDURL: fmt.Sprintf("%s%s", defaultBaseURL, defaultContainerIDURL),
		regionURL:      fmt.Sprintf("%s%s", defaultBaseURL, defaultRegionURL),
		projectIDURL:   fmt.Sprintf("%s%s", defaultBaseURL, defaultProjectID),
		timeout:        defaultTimeout,
	}
}

// GetMetaData returns the container's metadata
func GetMetaData(config *Config) *Metadata {
	wg := sync.WaitGroup{}
	wg.Add(3)
	httpClient := &http.Client{
		Timeout: config.timeout,
	}
	metadata := &Metadata{}
	go func() {
		metadata.containerID = getContainerID(httpClient, config)
		wg.Done()
	}()
	go func() {
		metadata.region = getRegion(httpClient, config)
		wg.Done()
	}()
	go func() {
		metadata.projectID = getProjectID(httpClient, config)
		wg.Done()
	}()
	wg.Wait()
	return metadata
}

func getContainerID(httpClient *http.Client, config *Config) *info {
	return &info{
		tagName: "container_id",
		value:   getSingleMetadata(httpClient, config.containerIDURL),
	}
}

func getRegion(httpClient *http.Client, config *Config) *info {
	value := getSingleMetadata(httpClient, config.regionURL)
	tokens := strings.Split(value, "/")
	return &info{
		tagName: "location",
		value:   tokens[len(tokens)-1],
	}
}

func getProjectID(httpClient *http.Client, config *Config) *info {
	return &info{
		tagName: "project_id",
		value:   getSingleMetadata(httpClient, config.projectIDURL),
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
		log.Error("unable to get the requested metadata, defaulting to unknown")
		return "unknown"
	}
	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Error("unable to read metadata body, defaulting to unknown")
		return "unknown"
	}
	return strings.ToLower(string(data))
}
