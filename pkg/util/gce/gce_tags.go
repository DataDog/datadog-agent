// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build gce

package gce

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GetTags gets the tags from the GCE api
func GetTags() ([]string, error) {
	tags := []string{}

	metadataResponse, err := getResponse(metadataURL + "/instance")
	if err != nil {
		return tags, err
	}

	metadata := gceMetadata{}

	err = json.Unmarshal([]byte(metadataResponse), &metadata)
	if err != nil {
		return tags, err
	}

	tags = metadata.Tags
	if metadata.Zone != "" {
		ts := strings.Split(metadata.Zone, "/")
		tags = append(tags, fmt.Sprintf("zone:%s", ts[len(ts)-1]))
	}
	if metadata.MachineType != "" {
		ts := strings.Split(metadata.MachineType, "/")
		tags = append(tags, fmt.Sprintf("instance-type:%s", ts[len(ts)-1]))
	}
	if metadata.Hostname != "" {
		tags = append(tags, fmt.Sprintf("internal-hostname:%s", metadata.Hostname))
	}
	if metadata.ID != 0 {
		tags = append(tags, fmt.Sprintf("instance-id:%d", metadata.ID))
	}
	if metadata.ProjectID != 0 {
		tags = append(tags, fmt.Sprintf("project:%d", metadata.ProjectID))
	}
	if metadata.NumericProjectID != 0 {
		tags = append(tags, fmt.Sprintf("numeric_project_id:%d", metadata.NumericProjectID))
	}

	return tags, nil
}
