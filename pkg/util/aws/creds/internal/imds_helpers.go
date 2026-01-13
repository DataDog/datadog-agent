// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Ec2IMDSVersionConfig is an enum to determine how to interact with the IMDSv2 option
type Ec2IMDSVersionConfig int

// Enum values for Ec2IMDSVersionConfig
const (
	ImdsV1 Ec2IMDSVersionConfig = iota
	ImdsAllVersions
	ImdsV2
)

// V2Allowed returns true if the agent is allowed to use IMDSv2
func (v Ec2IMDSVersionConfig) V2Allowed() bool {
	return v == ImdsAllVersions || v == ImdsV2
}

// V2Only returns true if the agent is forced to use IMDSv2
func (v Ec2IMDSVersionConfig) V2Only() bool {
	return v == ImdsV2
}

// GetMetadataItemWithMaxLength returns the metadata item at the given endpoint with a maximum length
func GetMetadataItemWithMaxLength(ctx context.Context, endpoint string, allowedIMDSVersions Ec2IMDSVersionConfig, updateMetadataSource bool) (string, error) {
	result, err := GetMetadataItem(ctx, endpoint, allowedIMDSVersions, updateMetadataSource)
	if err != nil {
		return result, err
	}

	maxLength := pkgconfigsetup.Datadog().GetInt("metadata_endpoints_max_hostname_size")
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

// GetMetadataItem returns the metadata item at the given endpoint
func GetMetadataItem(ctx context.Context, endpoint string, allowedIMDSVersions Ec2IMDSVersionConfig, updateMetadataSource bool) (string, error) {
	if !configutils.IsCloudProviderEnabled(CloudProviderName, pkgconfigsetup.Datadog()) {
		return "", errors.New("cloud provider is disabled by configuration")
	}

	return DoHTTPRequest(ctx, MetadataURL+endpoint, allowedIMDSVersions, updateMetadataSource)
}

// UseIMDSv2 returns true if the agent should use IMDSv2
func UseIMDSv2() Ec2IMDSVersionConfig {
	if pkgconfigsetup.Datadog().GetBool("ec2_prefer_imdsv2") || pkgconfigsetup.Datadog().GetBool("ec2_imdsv2_transition_payload_enabled") {
		return ImdsAllVersions
	}
	// if nothing indicates to use IMDSv2, we default to IMDSv1
	return ImdsV1
}

// DoHTTPRequest performs an HTTP request to the given url with proper ec2 headers
func DoHTTPRequest(ctx context.Context, url string, allowedIMDSVersions Ec2IMDSVersionConfig, updateMetadataSource bool) (string, error) {
	source := MetadataSourceIMDSv1
	headers := map[string]string{}
	if allowedIMDSVersions.V2Allowed() {
		tokenValue, err := getToken().Get(ctx)
		if err != nil {
			if allowedIMDSVersions.V2Only() {
				return "", fmt.Errorf("could not fetch token from IMDSv2: %s", err)
			}
			log.Debugf("ec2_prefer_imdsv2 is set to true in the configuration but the agent was unable to proceed: %s", err)
		} else {
			headers["X-aws-ec2-metadata-token"] = tokenValue
			if !allowedIMDSVersions.V2Only() {
				source = MetadataSourceIMDSv2
			}
		}
	}
	res, err := httputils.Get(ctx, url, headers, time.Duration(pkgconfigsetup.Datadog().GetInt("ec2_metadata_timeout"))*time.Millisecond, pkgconfigsetup.Datadog())
	// We don't want to register the source when we force imdsv2
	if err == nil && !allowedIMDSVersions.V2Only() && updateMetadataSource {
		SetCloudProviderSource(source)
	}
	return res, err
}
