// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package credentials

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type awsStore struct{}

func (s *awsStore) get(key string) (string, error) {
	ssmClient, err := clients.GetAWSSSMClient()
	if err != nil {
		return "", err
	}

	withDecription := true
	output, err := ssmClient.GetParameter(context.Background(), &ssm.GetParameterInput{Name: &key, WithDecryption: &withDecription})
	if err != nil {
		return "", fmt.Errorf("failed to get SSM parameter '%s', err: %w", key, err)
	}

	return *output.Parameter.Value, nil
}
