// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

// Package creds holds aws creds fetching related files
package creds

import (
	"context"
	"encoding/json"
	"fmt"

	ec2internal "github.com/DataDog/datadog-agent/pkg/util/aws/creds/internal"
)

// SecurityCredentials represents AWS security credentials from the EC2 instance metadata service.
// The Token field contains the session token (also known as SessionToken in AWS SDK terminology).
type SecurityCredentials struct {
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	Token           string `json:"Token"` // Session token from IMDS (maps to AWS_SESSION_TOKEN)
}

// GetSecurityCredentials retrieves AWS security credentials from the EC2 instance metadata service.
// This function queries the IMDS to get temporary credentials associated with the instance's IAM role.
func GetSecurityCredentials(ctx context.Context) (*SecurityCredentials, error) {
	iamRole, err := getIAMRole(ctx)
	if err != nil {
		return nil, err
	}

	res, err := ec2internal.DoHTTPRequest(ctx, ec2internal.MetadataURL+"/iam/security-credentials/"+iamRole, ec2internal.UseIMDSv2(), true)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch EC2 API to get iam role: %s", err)
	}

	creds := &SecurityCredentials{}
	err = json.Unmarshal([]byte(res), creds)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshall json, %s", err)
	}
	return creds, nil
}

// getIAMRole retrieves the IAM role name associated with the EC2 instance
func getIAMRole(ctx context.Context) (string, error) {
	res, err := ec2internal.DoHTTPRequest(ctx, ec2internal.MetadataURL+"/iam/security-credentials/", ec2internal.UseIMDSv2(), true)
	if err != nil {
		return "", fmt.Errorf("unable to fetch EC2 API to get security credentials: %s", err)
	}

	return res, nil
}
