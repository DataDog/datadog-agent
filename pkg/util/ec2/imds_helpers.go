// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getToken(ctx context.Context) (string, time.Time, error) {
	tokenLifetime := time.Duration(config.Datadog.GetInt("ec2_metadata_token_lifetime")) * time.Second
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
		config.Datadog.GetDuration("ec2_metadata_timeout")*time.Millisecond)

	if err != nil {
		return "", time.Now(), err
	}
	return res, expirationDate, nil
}

func getMetadataItemWithMaxLength(ctx context.Context, endpoint string, maxLength int) (string, error) {
	result, err := getMetadataItem(ctx, endpoint)
	if err != nil {
		return result, err
	}
	if len(result) > maxLength {
		return "", fmt.Errorf("%v gave a response with length > to %v", endpoint, maxLength)
	}
	return result, err
}

func getMetadataItem(ctx context.Context, endpoint string) (string, error) {
	if !config.IsCloudProviderEnabled(CloudProviderName) {
		return "", fmt.Errorf("cloud provider is disabled by configuration")
	}

	return doHTTPRequest(ctx, metadataURL+endpoint)
}

func doHTTPRequest(ctx context.Context, url string) (string, error) {
	source := metadataSourceIMDSv1
	headers := map[string]string{}
	if config.Datadog.GetBool("ec2_prefer_imdsv2") {
		tokenValue, err := token.Get(ctx)
		if err != nil {
			log.Warnf("ec2_prefer_imdsv2 is set to true in the configuration but the agent was unable to proceed: %s", err)
		} else {
			headers["X-aws-ec2-metadata-token"] = tokenValue
			source = metadataSourceIMDSv2
		}
	}

	res, err := httputils.Get(ctx, url, headers, time.Duration(config.Datadog.GetInt("ec2_metadata_timeout"))*time.Millisecond)
	if err == nil {
		setCloudProviderSource(source)
	}
	return res, err
}
