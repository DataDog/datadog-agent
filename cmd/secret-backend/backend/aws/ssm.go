// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"

	"github.com/DataDog/datadog-secret-backend/secret"
)

// ssmClient is an interface that defines the methods we use from the ssm client
// As the AWS SDK doesn't provide a real mock, we'll have to make our own that
// matches this interface
type ssmClient interface {
	GetParametersByPath(ctx context.Context, params *ssm.GetParametersByPathInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error)
	GetParameters(ctx context.Context, params *ssm.GetParametersInput, optFns ...func(*ssm.Options)) (*ssm.GetParametersOutput, error)
}

// getSSMClient is a variable that holds the function to create a new ssmClient
// it will be overwritten in tests
var getSSMClient = func(cfg aws.Config) ssmClient {
	return ssm.NewFromConfig(cfg)
}

// SSMParameterStoreBackendConfig is the configuration for a AWS SSM backend
type SSMParameterStoreBackendConfig struct {
	Session       SessionBackendConfig `mapstructure:"aws_session"`
	BackendType   string               `mapstructure:"backend_type"`
	ParameterPath string               `mapstructure:"parameter_path"`
	Parameters    []string             `mapstructure:"parameters"`
}

// SSMParameterStoreBackend represents backend for AWS SSM
type SSMParameterStoreBackend struct {
	BackendID string
	Config    SSMParameterStoreBackendConfig
	Secret    map[string]string
}

// NewSSMParameterStoreBackend returns a new AWS SSM backend
func NewSSMParameterStoreBackend(backendID string, bc map[string]interface{}) (
	*SSMParameterStoreBackend, error) {

	backendConfig := SSMParameterStoreBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendID).
			Msg("failed to map backend configuration")
		return nil, err
	}

	secretValue := make(map[string]string, 0)

	cfg, err := NewConfigFromBackendConfig(backendConfig.Session)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendID).
			Msg("failed to initialize aws session")
		return nil, err
	}
	client := getSSMClient(*cfg)

	// GetParametersByPath
	if backendConfig.ParameterPath != "" {
		input := &ssm.GetParametersByPathInput{
			Path:           &backendConfig.ParameterPath,
			Recursive:      aws.Bool(true),
			WithDecryption: aws.Bool(true),
		}

		pager := ssm.NewGetParametersByPathPaginator(client, input)
		for pager.HasMorePages() {
			out, err := pager.NextPage(context.TODO())
			if err != nil {
				log.Error().Err(err).
					Str("backend_id", backendID).
					Str("backend_type", backendConfig.BackendType).
					Str("parameter_path", backendConfig.ParameterPath).
					Strs("parameters", backendConfig.Parameters).
					Str("aws_access_key_id", backendConfig.Session.AccessKeyID).
					Str("aws_profile", backendConfig.Session.Profile).
					Str("aws_region", backendConfig.Session.Region).
					Msg("failed to retrieve parameters from path")
				return nil, err
			}

			for _, parameter := range out.Parameters {
				secretValue[*parameter.Name] = *parameter.Value
			}
		}
	}

	// GetParameters
	if len(backendConfig.Parameters) > 0 {
		input := &ssm.GetParametersInput{
			Names:          backendConfig.Parameters,
			WithDecryption: aws.Bool(true),
		}
		out, err := client.GetParameters(context.TODO(), input)
		if err != nil {
			log.Error().Err(err).
				Str("backend_id", backendID).
				Str("backend_type", backendConfig.BackendType).
				Strs("parameters", backendConfig.Parameters).
				Str("aws_access_key_id", backendConfig.Session.AccessKeyID).
				Str("aws_profile", backendConfig.Session.Profile).
				Str("aws_region", backendConfig.Session.Region).
				Msg("failed to retrieve parameters")
			return nil, err
		}

		// handle invalid parameters?
		for _, parameter := range out.Parameters {
			secretValue[*parameter.Name] = *parameter.Value
		}
	}

	backend := &SSMParameterStoreBackend{
		BackendID: backendID,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

// GetSecretOutput returns a the value for a specific secret
func (b *SSMParameterStoreBackend) GetSecretOutput(secretKey string) secret.Output {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.Output{Value: &val, Error: nil}
	}
	es := secret.ErrKeyNotFound.Error()

	log.Error().
		Str("backend_id", b.BackendID).
		Str("backend_type", b.Config.BackendType).
		Strs("parameters", b.Config.Parameters).
		Str("parameter_path", b.Config.ParameterPath).
		Str("secret_key", secretKey).
		Msg("failed to retrieve parameters")
	return secret.Output{Value: nil, Error: &es}
}
