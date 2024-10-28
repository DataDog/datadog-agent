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

// IMDSv2Action is an enum to determine how to interact with the IMDSv2 option
type IMDSv2Action int

const (
	disableIMDSv2 IMDSv2Action = iota
	useIMDSv2
	forceIMDSv2
)

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

func getMetadataItemWithMaxLength(ctx context.Context, endpoint string, action IMDSv2Action) (string, error) {
	result, err := getMetadataItem(ctx, endpoint, action)
	if err != nil {
		return result, err
	}

	maxLength := pkgconfigsetup.Datadog().GetInt("metadata_endpoints_max_hostname_size")
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getMetadataItem(ctx context.Context, endpoint string, action IMDSv2Action) (string, error) {
	if !pkgconfigsetup.IsCloudProviderEnabled(CloudProviderName, pkgconfigsetup.Datadog()) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	return doHTTPRequest(ctx, metadataURL+endpoint, action)
}

// UseIMDSv2 returns true if the agent should use IMDSv2
func UseIMDSv2(force bool, disable bool) IMDSv2Action {
	// if force is set, we use IMDSv2, highest priority
	if force {
		return forceIMDSv2
		// if disable is set, we disable IMDSv2
	} else if disable {
		return disableIMDSv2
		// if ec2_prefer_imdsv2 is set, we use IMDSv2
	} else if pkgconfigsetup.Datadog().GetBool("ec2_prefer_imdsv2") {
		return useIMDSv2
	}
	// if nothing indicates to use IMDSv2, we default to IMDSv1
	return disableIMDSv2
}

func doHTTPRequest(ctx context.Context, url string, action IMDSv2Action) (string, error) {
	source := metadataSourceIMDSv1
	headers := map[string]string{}
	if action >= useIMDSv2 {
		tokenValue, err := token.Get(ctx)
		if err != nil {
			if action == forceIMDSv2 {
				return "", fmt.Errorf("could not fetch token from IMDSv2")
			}
			log.Warnf("ec2_prefer_imdsv2 is set to true in the configuration but the agent was unable to proceed: %s", err)
		} else {
			headers["X-aws-ec2-metadata-token"] = tokenValue
			if action != forceIMDSv2 {
				source = metadataSourceIMDSv2
			}
		}
	}
	res, err := httputils.Get(ctx, url, headers, time.Duration(pkgconfigsetup.Datadog().GetInt("ec2_metadata_timeout"))*time.Millisecond, pkgconfigsetup.Datadog())
	// We don't want to register the source when we force imdsv2
	if err == nil && action != forceIMDSv2 {
		setCloudProviderSource(source)
	}
	return res, err
}
