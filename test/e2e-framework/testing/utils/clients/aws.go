// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clients

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	awsTimeout = 5 * time.Second
)

var (
	initLock     = sync.Mutex{}
	awsConfig    *aws.Config
	awsSSMClient *ssm.Client
)

// GetAWSSSMClient returns an aws SSM client
func GetAWSSSMClient() (*ssm.Client, error) {
	initLock.Lock()
	defer initLock.Unlock()

	if awsSSMClient != nil {
		return awsSSMClient, nil
	}

	cfg, err := getAWSConfig()
	if err != nil {
		return nil, err
	}

	awsSSMClient = ssm.NewFromConfig(*cfg)
	return awsSSMClient, nil
}

func getAWSConfig() (*aws.Config, error) {
	if awsConfig != nil {
		return awsConfig, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), awsTimeout)
	defer cancel()

	// https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/retries-timeouts/
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRetryer(func() aws.Retryer {
		return retry.AddWithMaxAttempts(retry.NewStandard(), 5)
	}))
	if err != nil {
		return nil, err
	}

	awsConfig = &cfg
	return awsConfig, nil
}
