// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/mitchellh/mapstructure"

	"github.com/DataDog/datadog-secret-backend/secret"
)

// ssmClient is an interface that defines the methods we use from the ssm client
// As the AWS SDK doesn't provide a real mock, we'll have to make our own that
// matches this interface
type ssmClient interface {
	GetParametersByPath(ctx context.Context, params *ssm.GetParametersByPathInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error)
	GetParameters(ctx context.Context, params *ssm.GetParametersInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersOutput, error)
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// getSSMClient is a variable that holds the function to create a new ssmClient
// it will be overwritten in tests
var getSSMClient = func(cfg aws.Config) ssmClient {
	return ssm.NewFromConfig(cfg)
}

// SSMParameterStoreBackendConfig is the configuration for a AWS SSM backend
type SSMParameterStoreBackendConfig struct {
	Session     SessionBackendConfig `mapstructure:"aws_session"`
	BackendType string               `mapstructure:"backend_type"`
}

// SSMParameterStoreBackend represents backend for AWS SSM
type SSMParameterStoreBackend struct {
	Config SSMParameterStoreBackendConfig
	Client ssmClient
}

// NewSSMParameterStoreBackend returns a new AWS SSM backend
func NewSSMParameterStoreBackend(bc map[string]interface{}) (*SSMParameterStoreBackend, error) {
	backendConfig := SSMParameterStoreBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to map backend configuration: %s", err)
	}

	cfg, err := NewConfigFromBackendConfig(backendConfig.Session)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize aws session: %s", err)
	}
	client := getSSMClient(*cfg)

	backend := &SSMParameterStoreBackend{
		Config: backendConfig,
		Client: client,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *SSMParameterStoreBackend) GetSecretOutput(secretKey string) secret.Output {
	input := &ssm.GetParameterInput{
		Name:           &secretKey,
		WithDecryption: aws.Bool(true),
	}

	out, err := b.Client.GetParameter(context.TODO(), input)
	if err != nil {
		es := err.Error()
		return secret.Output{Value: nil, Error: &es}
	}

	if out.Parameter == nil || out.Parameter.Value == nil {
		es := "parameter value is nil"
		return secret.Output{Value: nil, Error: &es}
	}

	return secret.Output{Value: out.Parameter.Value, Error: nil}
}
