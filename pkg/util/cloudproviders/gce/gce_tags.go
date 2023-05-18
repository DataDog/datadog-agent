// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build gce

package gce

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tagsCacheKey = cache.BuildAgentKey("gce", "GetTags")
)

type gceMetadata struct {
	Instance gceInstanceMetadata
	Project  gceProjectMetadata
}

type gceInstanceMetadata struct {
	ID          int64
	Tags        []string
	Zone        string
	MachineType string
	Hostname    string
	Attributes  map[string]string
}

type gceProjectMetadata struct {
	ProjectID        string
	NumericProjectID int64
}

func getCachedTags(err error) ([]string, error) {
	if gceTags, found := cache.Cache.Get(tagsCacheKey); found {
		log.Infof("unable to get tags from gce, returning cached tags: %s", err)
		return gceTags.([]string), nil
	}
	return nil, log.Warnf("unable to get tags from gce and cache is empty: %s", err)
}

// GetTags gets the tags from the GCE api
func GetTags(ctx context.Context) ([]string, error) {

	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return nil, fmt.Errorf("cloud provider is disabled by configuration")
	}

	metadataResponse, err := getResponse(ctx, metadataURL+"/?recursive=true")
	if err != nil {
		return getCachedTags(err)
	}

	metadata := gceMetadata{}

	err = json.Unmarshal([]byte(metadataResponse), &metadata)
	if err != nil {
		return getCachedTags(err)
	}

	tags := metadata.Instance.Tags
	if metadata.Instance.Zone != "" {
		ts := strings.Split(metadata.Instance.Zone, "/")
		tags = append(tags, fmt.Sprintf("zone:%s", ts[len(ts)-1]))
	}
	if metadata.Instance.MachineType != "" {
		ts := strings.Split(metadata.Instance.MachineType, "/")
		tags = append(tags, fmt.Sprintf("instance-type:%s", ts[len(ts)-1]))
	}
	if metadata.Instance.Hostname != "" {
		tags = append(tags, fmt.Sprintf("internal-hostname:%s", metadata.Instance.Hostname))
	}
	if metadata.Instance.ID != 0 {
		tags = append(tags, fmt.Sprintf("instance-id:%d", metadata.Instance.ID))
	}
	if metadata.Project.ProjectID != "" {
		tags = append(tags, fmt.Sprintf("project:%s", metadata.Project.ProjectID))
		if config.Datadog.GetBool("gce_send_project_id_tag") {
			tags = append(tags, fmt.Sprintf("project_id:%s", metadata.Project.ProjectID))
		}
	}
	if metadata.Project.NumericProjectID != 0 {
		tags = append(tags, fmt.Sprintf("numeric_project_id:%d", metadata.Project.NumericProjectID))
	}

	if metadata.Instance.Attributes != nil {
		for k, v := range metadata.Instance.Attributes {
			if !isAttributeExcluded(k) {
				tags = append(tags, fmt.Sprintf("%s:%s", k, v))
			}
		}
	}

	// save tags to the cache in case we exceed quotas later
	cache.Cache.Set(tagsCacheKey, tags, cache.NoExpiration)

	return tags, nil
}

// isAttributeExcluded returns whether the attribute key should be excluded from the tags
func isAttributeExcluded(attr string) bool {

	excludedAttributes := config.Datadog.GetStringSlice("exclude_gce_tags")
	for _, excluded := range excludedAttributes {
		if attr == excluded {
			return true
		}
	}
	return false
}
