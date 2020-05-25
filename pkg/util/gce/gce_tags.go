// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build gce

package gce

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
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

// GetTags gets the tags from the GCE api
func GetTags() ([]string, error) {
	tags := []string{}

	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return tags, fmt.Errorf("cloud provider is disabled by configuration")
	}

	metadataResponse, err := getResponse(metadataURL + "/?recursive=true")
	if err != nil {
		return tags, err
	}

	metadata := gceMetadata{}

	err = json.Unmarshal([]byte(metadataResponse), &metadata)
	if err != nil {
		return tags, err
	}

	tags = metadata.Instance.Tags
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
