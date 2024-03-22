package aws

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog/log"

	"github.com/rapdev-io/datadog-secret-backend/secret"
)

type AwsSsmParameterStoreBackendConfig struct {
	AwsSession    AwsSessionBackendConfig `mapstructure:"aws_session"`
	BackendType   string                  `mapstructure:"backend_type"`
	ParameterPath string                  `mapstructure:"parameter_path"`
	Parameters    []string                `mapstructure:"parameters"`
}

type AwsSsmParameterStoreBackend struct {
	BackendId string
	Config    AwsSsmParameterStoreBackendConfig
	Secret    map[string]string
}

func NewAwsSsmParameterStoreBackend(backendId string, bc map[string]interface{}) (
	*AwsSsmParameterStoreBackend, error) {

	backendConfig := AwsSsmParameterStoreBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("failed to map backend configuration")
		return nil, err
	}

	secretValue := make(map[string]string, 0)

	cfg, err := NewAwsConfigFromBackendConfig(backendId, backendConfig.AwsSession)
	if err != nil {
		log.Error().Err(err).Str("backend_id", backendId).
			Msg("failed to initialize aws session")
		return nil, err
	}
	client := ssm.NewFromConfig(*cfg)

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
					Str("backend_id", backendId).
					Str("backend_type", backendConfig.BackendType).
					Str("parameter_path", backendConfig.ParameterPath).
					Strs("parameters", backendConfig.Parameters).
					Str("aws_access_key_id", backendConfig.AwsSession.AwsAccessKeyId).
					Str("aws_profile", backendConfig.AwsSession.AwsProfile).
					Str("aws_region", backendConfig.AwsSession.AwsRegion).
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
				Str("backend_id", backendId).
				Str("backend_type", backendConfig.BackendType).
				Strs("parameters", backendConfig.Parameters).
				Str("aws_access_key_id", backendConfig.AwsSession.AwsAccessKeyId).
				Str("aws_profile", backendConfig.AwsSession.AwsProfile).
				Str("aws_region", backendConfig.AwsSession.AwsRegion).
				Msg("failed to retrieve parameters")
			return nil, err
		}

		// handle invalid parameters?
		for _, parameter := range out.Parameters {
			secretValue[*parameter.Name] = *parameter.Value
		}
	}

	backend := &AwsSsmParameterStoreBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

func (b *AwsSsmParameterStoreBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.Error().
		Str("backend_id", b.BackendId).
		Str("backend_type", b.Config.BackendType).
		Strs("parameters", b.Config.Parameters).
		Str("parameter_path", b.Config.ParameterPath).
		Str("secret_key", secretKey).
		Msg("failed to retrieve parameters")
	return secret.SecretOutput{Value: nil, Error: &es}
}
