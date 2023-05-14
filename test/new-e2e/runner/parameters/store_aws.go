// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parameters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type awsStore struct {
	prefix string
}

func NewAWSStore(prefix string) Store {
	return newStore(newCachingStore(awsStore{
		prefix: prefix,
	}))
}

// Get returns parameter value.
// For AWS Store, parameter key is lowered and added to prefix
func (s awsStore) get(key string) (string, error) {
	ssmClient, err := clients.GetAWSSSMClient()
	if err != nil {
		return "", err
	}

	key = strings.ToLower(s.prefix + key)
	withDecription := true
	output, err := ssmClient.GetParameter(context.Background(), &ssm.GetParameterInput{Name: &key, WithDecryption: &withDecription})
	if err != nil {
		var notFoundError *ssmTypes.ParameterNotFound
		if errors.As(err, &notFoundError) {
			return "", ParameterNotFoundError{key: key}
		}

		return "", fmt.Errorf("failed to get SSM parameter '%s', err: %w", key, err)
	}

	return *output.Parameter.Value, nil
}
