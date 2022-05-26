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

const defaultBaseUrl = "http://metadata.google.internal/computeMetadata/v1"
const defaultContainerIDUrl = "/instance/id"
const defaultRegionUrl = "/instance/region"
const defaultProjectID = "/project/project-id"
const defaultTimeout = 300 * time.Millisecond

type Config struct {
	ContainerIDUrl string
	RegionUrl      string
	ProjectIDUrl   string
	timeout        time.Duration
}

type MetadataInfo struct {
	tagName string
	value   string
}

type Metadata struct {
	ContainerID *MetadataInfo
	Region      *MetadataInfo
	ProjectID   *MetadataInfo
}

func (metadata *Metadata) TagMap() map[string]string {
	tagMap := map[string]string{}
	if metadata.ContainerID != nil {
		tagMap[metadata.ContainerID.tagName] = metadata.ContainerID.value
	}
	if metadata.Region != nil {
		tagMap[metadata.Region.tagName] = metadata.Region.value
	}
	if metadata.ProjectID != nil {
		tagMap[metadata.ProjectID.tagName] = metadata.ProjectID.value
	}
	return tagMap
}

func GetDefaultConfig() *Config {
	return &Config{
		ContainerIDUrl: fmt.Sprintf("%s%s", defaultBaseUrl, defaultContainerIDUrl),
		RegionUrl:      fmt.Sprintf("%s%s", defaultBaseUrl, defaultRegionUrl),
		ProjectIDUrl:   fmt.Sprintf("%s%s", defaultBaseUrl, defaultProjectID),
		timeout:        defaultTimeout,
	}
}

func GetMetaData(config *Config) *Metadata {
	wg := sync.WaitGroup{}
	metadata := &Metadata{}
	wg.Add(3)
	go func() {
		metadata.ContainerID = getContainerID(config)
		wg.Done()
	}()
	go func() {
		metadata.Region = getRegion(config)
		wg.Done()
	}()
	go func() {
		metadata.ProjectID = getProjectID(config)
		wg.Done()
	}()
	// make extra sure that we will not wait for this waig group forever
	timing.WaitWithTimeout(&wg, config.timeout)
	return metadata
}

func getContainerID(config *Config) *MetadataInfo {
	return &MetadataInfo{
		tagName: "containerid",
		value:   getSingleMetadata(config.ContainerIDUrl, config.timeout),
	}
}

func getRegion(config *Config) *MetadataInfo {
	value := getSingleMetadata(config.RegionUrl, config.timeout)
	tokens := strings.Split(value, "/")
	return &MetadataInfo{
		tagName: "region",
		value:   tokens[len(tokens)-1],
	}
}

func getProjectID(config *Config) *MetadataInfo {
	return &MetadataInfo{
		tagName: "projectid",
		value:   getSingleMetadata(config.ProjectIDUrl, config.timeout),
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
