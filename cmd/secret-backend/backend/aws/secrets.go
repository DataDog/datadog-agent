package aws

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/mitchellh/mapstructure"
	"github.com/rapdev-io/datadog-secret-backend/secret"

	log "github.com/sirupsen/logrus"
)

type AwsSecretsManagerBackendConfig struct {
	AwsSession  AwsSessionBackendConfig `mapstructure:"aws_session"`
	BackendType string                  `mapstructure:"backend_type"`
	SecretId    string                  `mapstructure:"secret_id"`
}

type AwsSecretsManagerBackend struct {
	BackendId string
	Config    AwsSecretsManagerBackendConfig
	Secret    map[string]string
}

func NewAwsSecretsManagerBackend(backendId string, bc map[string]interface{}) (
	*AwsSecretsManagerBackend, error) {

	backendConfig := AwsSecretsManagerBackendConfig{}
	err := mapstructure.Decode(bc, &backendConfig)
	if err != nil {
		log.WithError(err).Error("failed to map backend configuration")
		return nil, err
	}

	cfg, err := NewAwsConfigFromBackendConfig(backendId, backendConfig.AwsSession)
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id": backendId,
		}).WithError(err).Error("failed to initialize aws session")
		return nil, err
	}
	client := secretsmanager.NewFromConfig(*cfg)

	// GetSecretValue
	input := &secretsmanager.GetSecretValueInput{
		SecretId: &backendConfig.SecretId,
	}
	out, err := client.GetSecretValue(context.TODO(), input)
	if err != nil {
		log.WithFields(log.Fields{
			"backend_id":        backendId,
			"backend_type":      backendConfig.BackendType,
			"secret_id":         backendConfig.SecretId,
			"aws_access_key_id": backendConfig.AwsSession.AwsAccessKeyId,
			"aws_profile":       backendConfig.AwsSession.AwsProfile,
		}).WithError(err).Error("failed to retrieve secret value")
		return nil, err
	}

	secretValue := make(map[string]string, 0)
	if err := json.Unmarshal([]byte(*out.SecretString), &secretValue); err != nil {
		log.WithFields(log.Fields{
			"backend_id":        backendId,
			"backend_type":      backendConfig.BackendType,
			"secret_id":         backendConfig.SecretId,
			"aws_access_key_id": backendConfig.AwsSession.AwsAccessKeyId,
			"aws_profile":       backendConfig.AwsSession.AwsProfile,
		}).WithError(err).Error("failed to retrieve secret value")
		return nil, err
	}

	backend := &AwsSecretsManagerBackend{
		BackendId: backendId,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return backend, nil
}

func (b *AwsSecretsManagerBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.WithFields(log.Fields{
		"backend_id":   b.BackendId,
		"backend_type": b.Config.BackendType,
		"secret_id":    b.Config.SecretId,
		"secret_key":   secretKey,
	}).Error("backend does not provide secret key")
	return secret.SecretOutput{Value: nil, Error: &es}
}
