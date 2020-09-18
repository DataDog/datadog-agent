// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package azure

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
)

type azureInstanceMetadata struct {
	VMID          string `json:"vmId"`
	Zone          string `json:"zone"`
	VMSize        string `json:"vmSize"`
	ResourceGroup string `json:"resourceGroupName"`
}

// GetTags gets the tags from the Azure api
func GetTags() ([]string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return nil, fmt.Errorf("cloud provider is disabled by configuration")
	}

	tags := []string{}

	metadataResponse, err := getResponse(metadataURL + "/metadata/instance/compute?api-version=2017-08-01")
	if err != nil {
		return nil, fmt.Errorf("unable to query metadata endpoint: %s", err)
	}

	metadata := azureInstanceMetadata{}

	err = json.Unmarshal([]byte(metadataResponse), &metadata)
	if err != nil {
		return nil, err
	}

	if metadata.VMID != "" {
		tags = append(tags, fmt.Sprintf("vm-id:%s", metadata.VMID))
	}
	if metadata.Zone != "" {
		tags = append(tags, fmt.Sprintf("zone:%s", metadata.Zone))
	}
	if metadata.VMSize != "" {
		tags = append(tags, fmt.Sprintf("vm-size:%s", metadata.VMSize))
	}
	if metadata.ResourceGroup != "" {
		tags = append(tags, fmt.Sprintf("resource-group:%s", metadata.ResourceGroup))
	}

	return tags, nil
}
