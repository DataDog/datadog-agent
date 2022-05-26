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

	"github.com/DataDog/datadog-agent/cmd/serverless-init/timing"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultBaseURL = "http://metadata.google.internal/computeMetadata/v1"
const defaultContainerIDURL = "/instance/id"
const defaultRegionURL = "/instance/region"
const defaultProjectID = "/project/project-id"
const defaultTimeout = 300 * time.Millisecond

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

type Metadata struct {
	containerID *info
	region      *info
	projectID   *info
}

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

func GetDefaultConfig() *Config {
	return &Config{
		containerIDURL: fmt.Sprintf("%s%s", defaultBaseURL, defaultContainerIDURL),
		regionURL:      fmt.Sprintf("%s%s", defaultBaseURL, defaultRegionURL),
		projectIDURL:   fmt.Sprintf("%s%s", defaultBaseURL, defaultProjectID),
		timeout:        defaultTimeout,
	}
}

func GetMetaData(config *Config) *Metadata {
	wg := sync.WaitGroup{}
	metadata := &Metadata{}
	wg.Add(3)
	go func() {
		metadata.containerID = getContainerID(config)
		wg.Done()
	}()
	go func() {
		metadata.region = getRegion(config)
		wg.Done()
	}()
	go func() {
		metadata.projectID = getProjectID(config)
		wg.Done()
	}()
	// make extra sure that we will not wait for this waig group forever
	timing.WaitWithTimeout(&wg, config.timeout)
	return metadata
}

func getContainerID(config *Config) *info {
	return &info{
		tagName: "containerid",
		value:   getSingleMetadata(config.containerIDURL, config.timeout),
	}
}

func getRegion(config *Config) *info {
	value := getSingleMetadata(config.regionURL, config.timeout)
	tokens := strings.Split(value, "/")
	return &info{
		tagName: "region",
		value:   tokens[len(tokens)-1],
	}
}

func getProjectID(config *Config) *info {
	return &info{
		tagName: "projectid",
		value:   getSingleMetadata(config.projectIDURL, config.timeout),
	}
}

func getSingleMetadata(url string, timeout time.Duration) string {
	client := &http.Client{
		Timeout: timeout,
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Error("unable to build the metadata request, defaulting to unknown")
		return "unknown"
	}
	req.Header.Add("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil {
		log.Error("unable to get the requested metadata, defaulting to unknown")
		return "unknown"
	}
	data, _ := ioutil.ReadAll(res.Body)
	return strings.ToLower(string(data))
}
