// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ec2internal contains internal helpers for AWS IMDS (Instance Metadata Service).
// Note: This duplicates pkg/util/ec2/internal because Go's internal package rules
// prevent importing that package from outside the ec2 directory tree.
package ec2internal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Metadata sources
const (
	MetadataSourceNone = iota
	MetadataSourceUUID
	MetadataSourceDMI
	MetadataSourceIMDSv1
	MetadataSourceIMDSv2
)

const (
	// CloudProviderName contains the inventory name of for EC2
	CloudProviderName = "AWS"
	// DMIBoardVendor contains the DMI board vendor for EC2
	DMIBoardVendor = "Amazon EC2"
)

// Use variables to ease mocking in tests
var (
	MetadataURL         = "http://169.254.169.254/latest/meta-data"
	TokenURL            = "http://169.254.169.254/latest/api/token"
	InstanceIdentityURL = "http://169.254.169.254/latest/dynamic/instance-identity/document/"

	CurrentMetadataSource      = MetadataSourceNone
	currentMetadataSourceMutex sync.Mutex

	Token              *httputils.APIToken
	tokenOnce          sync.Once
	tokenRenewalWindow = 15 * time.Second
)

// getToken returns the APIToken instance, initializing it lazily on first use.
// This avoids using init() which breaks initialization order.
func getToken() *httputils.APIToken {
	tokenOnce.Do(func() {
		Token = httputils.NewAPIToken(GetToken)
	})
	return Token
}

// SetCloudProviderSource set the best source available for EC2 metadata to the inventories payload.
//
// The different sources that can be used to know if we are running on EC2. This data is registered in the
// inventories metadata payload.
//
// We current have 3 ways to know we're on EC2 (if one fails we fallback to the next):
// - we succeed in reaching IMDS v1 or v2 metadata API.
// - the DMI information match EC2 and we can get the instanceID from it.
// - the product UUID or hypervisor UUID match EC2 (we know we're on EC2 but can't fetch the instance ID).
//
// Since some ways can temporary fail, we always register the "best" that worked at some point. This is mainly aimed at
// IMDS which is sometimes unavailable at startup.
func SetCloudProviderSource(source int) {
	currentMetadataSourceMutex.Lock()
	defer currentMetadataSourceMutex.Unlock()

	if source <= CurrentMetadataSource {
		return
	}

	CurrentMetadataSource = source
}

// GetSourceName returns the source used to pull information for EC2
func GetSourceName() string {
	currentMetadataSourceMutex.Lock()
	defer currentMetadataSourceMutex.Unlock()

	switch CurrentMetadataSource {
	case MetadataSourceUUID:
		return "UUID"
	case MetadataSourceDMI:
		return "DMI"
	case MetadataSourceIMDSv1:
		return "IMDSv1"
	case MetadataSourceIMDSv2:
		return "IMDSv2"
	}
	return ""
}

// GetToken fetches a new token from the EC2 metadata service
func GetToken(ctx context.Context) (string, time.Time, error) {
	tokenLifetime := time.Duration(pkgconfigsetup.Datadog().GetInt("ec2_metadata_token_lifetime")) * time.Second
	// Set the local expiration date before requesting the metadata endpoint so the local expiration date will always
	// expire before the expiration date computed on the AWS side. The expiration date is set minus the renewal window
	// to ensure the token will be refreshed before it expires.
	expirationDate := time.Now().Add(tokenLifetime - tokenRenewalWindow)

	res, err := httputils.Put(ctx,
		TokenURL,
		map[string]string{
			"X-aws-ec2-metadata-token-ttl-seconds": strconv.Itoa(int(tokenLifetime.Seconds())),
		},
		nil,
		pkgconfigsetup.Datadog().GetDuration("ec2_metadata_timeout")*time.Millisecond, pkgconfigsetup.Datadog())
	if err != nil {
		return "", time.Now(), err
	}
	return res, expirationDate, nil
}

// EC2Identity holds the instances identity document
// nolint: revive
type EC2Identity struct {
	Region     string
	InstanceID string
	AccountID  string
}

// GetInstanceIdentity returns the instance identity document for the current instance
func GetInstanceIdentity(ctx context.Context) (*EC2Identity, error) {
	instanceIdentity := &EC2Identity{}
	res, err := DoHTTPRequest(ctx, InstanceIdentityURL, UseIMDSv2(), true)
	if err != nil {
		return instanceIdentity, fmt.Errorf("unable to fetch EC2 API to get identity: %s", err)
	}

	err = json.Unmarshal([]byte(res), &instanceIdentity)
	if err != nil {
		return instanceIdentity, fmt.Errorf("unable to unmarshall json, %s", err)
	}

	return instanceIdentity, nil
}

// GetInstanceDocument returns information about the local EC2 instance using the local AWS API
func GetInstanceDocument(ctx context.Context) (map[string]string, error) {
	res, err := DoHTTPRequest(ctx, InstanceIdentityURL, UseIMDSv2(), false)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch EC2 API to get instance information: %s", err)
	}

	info := map[string]string{}
	err = json.Unmarshal([]byte(res), &info)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshall json, %s", err)
	}

	return info, nil
}
