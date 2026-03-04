// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ec2

// Package creds holds aws creds fetching related files
package creds

import (
	"context"
	"errors"
)

// SecurityCredentials represents AWS security credentials.
// This stub version is available when not compiled with the ec2 build tag.
type SecurityCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	Token           string // Session token (maps to AWS_SESSION_TOKEN)
}

// GetSecurityCredentials is a no-op when not compiled with the ec2 build tag.
func GetSecurityCredentials(_ context.Context) (*SecurityCredentials, error) {
	return nil, errors.New("EC2 metadata service is not available (not compiled with ec2 build tag)")
}
