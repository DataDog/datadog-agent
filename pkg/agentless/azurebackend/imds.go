// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package azurebackend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// InstanceMetadata contains the metadata for the VM instance, including compute.
type InstanceMetadata struct {
	Compute *ComputeMetadata `json:"compute"`
}

// ComputeMetadata contains the compute metadata for the VM instance.
type ComputeMetadata struct {
	Location          string `json:"location"`
	Name              string `json:"name"`
	ResourceGroupName string `json:"resourceGroupName"`
	ResourceID        string `json:"resourceId"`
	SubscriptionID    string `json:"subscriptionId"`
	Zone              string `json:"zone"`
}

// GetInstanceMetadata returns the instance metadata from the Azure Instance Metadata Service.
func GetInstanceMetadata(ctx context.Context) (metadata InstanceMetadata, err error) {
	const imdsURL = "http://169.254.169.254/metadata/instance"
	const imdsVersion = "2021-12-13"

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, imdsURL, http.NoBody)
	req.Header.Add("Metadata", "True")
	q := req.URL.Query()
	q.Add("format", "json")
	q.Add("api-version", imdsVersion)
	req.URL.RawQuery = q.Encode()

	client := http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return metadata, fmt.Errorf("error querying instance metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return metadata, fmt.Errorf("error querying instance metadata: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return metadata, err
	}

	if err = json.Unmarshal(body, &metadata); err != nil {
		return metadata, err
	}
	if err != nil {
		return metadata, err
	}

	return metadata, nil
}
