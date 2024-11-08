// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"fmt"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	imdsInstanceID = "/instance-id"
	imdsHostname   = "/hostname"
	// This is used in ec2_tags.go which is behind the 'ec2' build flag
	imdsTags        = "/tags/instance" //nolint:unused
	imdsIPv4        = "/public-ipv4"
	imdsNetworkMacs = "/network/interfaces/macs"
)

// ec2IMDSVersionConfig is an enum to determine how to interact with the IMDSv2 option
type ec2IMDSVersionConfig int

const (
	imdsV1 ec2IMDSVersionConfig = iota
	imdsAllVersions
	imdsV2
)

func (v ec2IMDSVersionConfig) V2Allowed() bool {
	return v == imdsAllVersions || v == imdsV2
}

func (v ec2IMDSVersionConfig) V2Only() bool {
	return v == imdsV2
}

func getToken(ctx context.Context) (string, time.Time, error) {
	tokenLifetime := time.Duration(pkgconfigsetup.Datadog().GetInt("ec2_metadata_token_lifetime")) * time.Second
	// Set the local expiration date before requesting the metadata endpoint so the local expiration date will always
	// expire before the expiration date computed on the AWS side. The expiration date is set minus the renewal window
	// to ensure the token will be refreshed before it expires.
	expirationDate := time.Now().Add(tokenLifetime - tokenRenewalWindow)

	res, err := httputils.Put(ctx,
		tokenURL,
		map[string]string{
			"X-aws-ec2-metadata-token-ttl-seconds": fmt.Sprintf("%d", int(tokenLifetime.Seconds())),
		},
		nil,
		pkgconfigsetup.Datadog().GetDuration("ec2_metadata_timeout")*time.Millisecond, pkgconfigsetup.Datadog())
	if err != nil {
		return "", time.Now(), err
	}
	return res, expirationDate, nil
}

func getMetadataItemWithMaxLength(ctx context.Context, endpoint string, allowedIMDSVersions ec2IMDSVersionConfig, updateMetadataSource bool) (string, error) {
	result, err := getMetadataItem(ctx, endpoint, allowedIMDSVersions, updateMetadataSource)
	if err != nil {
		return result, err
	}

	maxLength := pkgconfigsetup.Datadog().GetInt("metadata_endpoints_max_hostname_size")
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getMetadataItem(ctx context.Context, endpoint string, allowedIMDSVersions ec2IMDSVersionConfig, updateMetadataSource bool) (string, error) {
	if !pkgconfigsetup.IsCloudProviderEnabled(CloudProviderName, pkgconfigsetup.Datadog()) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	return doHTTPRequest(ctx, metadataURL+endpoint, allowedIMDSVersions, updateMetadataSource)
}

// useIMDSv2 returns true if the agent should use IMDSv2
func useIMDSv2() ec2IMDSVersionConfig {
	if pkgconfigsetup.Datadog().GetBool("ec2_prefer_imdsv2") || pkgconfigsetup.Datadog().GetBool("ec2_imdsv2_transition_payload_enabled") {
		return imdsAllVersions
	}
	// if nothing indicates to use IMDSv2, we default to IMDSv1
	return imdsV1
}

func doHTTPRequest(ctx context.Context, url string, allowedIMDSVersions ec2IMDSVersionConfig, updateMetadataSource bool) (string, error) {
	source := metadataSourceIMDSv1
	headers := map[string]string{}
	if allowedIMDSVersions.V2Allowed() {
		tokenValue, err := token.Get(ctx)
		if err != nil {
			if allowedIMDSVersions.V2Only() {
				return "", fmt.Errorf("could not fetch token from IMDSv2")
			}
			log.Warnf("ec2_prefer_imdsv2 is set to true in the configuration but the agent was unable to proceed: %s", err)
		} else {
			headers["X-aws-ec2-metadata-token"] = tokenValue
			if !allowedIMDSVersions.V2Only() {
				source = metadataSourceIMDSv2
			}
		}
	}
	res, err := httputils.Get(ctx, url, headers, time.Duration(pkgconfigsetup.Datadog().GetInt("ec2_metadata_timeout"))*time.Millisecond, pkgconfigsetup.Datadog())
	// We don't want to register the source when we force imdsv2
	if err == nil && !allowedIMDSVersions.V2Only() && updateMetadataSource {
		setCloudProviderSource(source)
	}
	return res, err
}
