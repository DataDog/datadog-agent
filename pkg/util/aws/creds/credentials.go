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
	"io"
	"net/http"
	"os"

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

// GetECSSecurityCredentials retrieves AWS credentials from the ECS container credential endpoint.
// This is used for ECS Fargate and ECS EC2 tasks where credentials are provided via the
// container metadata service rather than EC2 IMDS.
// See: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-iam-roles.html
func GetECSSecurityCredentials(ctx context.Context) (*SecurityCredentials, error) {
	uri := os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")
	if uri == "" {
		rel := os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")
		if rel == "" {
			return nil, fmt.Errorf("neither AWS_CONTAINER_CREDENTIALS_FULL_URI nor AWS_CONTAINER_CREDENTIALS_RELATIVE_URI is set")
		}
		uri = "http://169.254.170.2" + rel
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECS credential request: %w", err)
	}

	// AWS_CONTAINER_AUTHORIZATION_TOKEN is required for AWS_CONTAINER_CREDENTIALS_FULL_URI
	if token := os.Getenv("AWS_CONTAINER_AUTHORIZATION_TOKEN"); token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ECS container credentials: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ECS credential endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read ECS credential response: %w", err)
	}

	creds := &SecurityCredentials{}
	if err := json.Unmarshal(body, creds); err != nil {
		return nil, fmt.Errorf("failed to parse ECS credential response: %w", err)
	}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return nil, fmt.Errorf("ECS credential response missing AccessKeyId or SecretAccessKey")
	}
	return creds, nil
}
