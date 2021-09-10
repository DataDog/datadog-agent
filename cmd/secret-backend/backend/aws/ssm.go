package aws

import (
	"context"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/mitchellh/mapstructure"
	"github.com/rapdev-io/datadog-secret-backend/secret"

	log "github.com/sirupsen/logrus"
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
		log.WithError(err).Error("failed to map backend configuration")
		return nil, err
	}

	secretValue := make(map[string]string, 0)

	cfg, err := NewAwsConfigFromBackendConfig(backendId, backendConfig.AwsSession)
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id": backendId,
		}).WithError(err).Error("failed to initialize aws session")
		return nil, err
	}
	client := ssm.NewFromConfig(*cfg)

	// GetParametersByPath
	if backendConfig.ParameterPath != "" {
		input := &ssm.GetParametersByPathInput{
			Path:           &backendConfig.ParameterPath,
			Recursive:      true,
			WithDecryption: true,
		}

		pager := ssm.NewGetParametersByPathPaginator(client, input)
		for pager.HasMorePages() {
			out, err := pager.NextPage(context.TODO())
			if err != nil {
				log.WithFields(log.Fields{
					"backend_id":        backendId,
					"backend_type":      backendConfig.BackendType,
					"parameter_path":    backendConfig.ParameterPath,
					"aws_access_key_id": backendConfig.AwsSession.AwsAccessKeyId,
					"aws_profile":       backendConfig.AwsSession.AwsProfile,
					"aws_region":        backendConfig.AwsSession.AwsRegion,
				}).WithError(err).Error("failed to retrieve parameters from path")
				return nil, err
			}

			for _, parameter := range out.Parameters {
				secretValue[*parameter.Name] = *parameter.Value
			}
		}
	}

	// GetParameters
	if len(backendConfig.Parameters) > 0 {

		input := &ssm.GetParametersInput{Names: backendConfig.Parameters, WithDecryption: true}
		out, err := client.GetParameters(context.TODO(), input)
		if err != nil {
			log.WithFields(log.Fields{
				"backend_id":        backendId,
				"backend_type":      backendConfig.BackendType,
				"parameters":        strings.Join(backendConfig.Parameters, ","),
				"aws_access_key_id": backendConfig.AwsSession.AwsAccessKeyId,
				"aws_profile":       backendConfig.AwsSession.AwsProfile,
				"aws_region":        backendConfig.AwsSession.AwsRegion,
			}).WithError(err).Error("failed to retrieve parameters")
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

	log.WithFields(log.Fields{
		"backend_id":   b.BackendId,
		"backend_type": b.Config.BackendType,
	}).Error("backend does not provide secret key")
	return secret.SecretOutput{Value: nil, Error: &es}
}
