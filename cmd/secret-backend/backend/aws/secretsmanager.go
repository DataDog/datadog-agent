package aws

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/rapdev-io/datadog-secret-backend/secret"

	log "github.com/sirupsen/logrus"
)

type AwsSecretsManagerBackend struct {
	BackendId string
	Client    *secretsmanager.Client
	Config    map[string]string
	Secret    map[string]string
}

func NewAwsSecretsManagerBackend(backendId string, backendConfig map[string]string) (
	*AwsSecretsManagerBackend, error) {

	cfg, err := NewAwsConfigFromBackendConfig(backendId, backendConfig)
	if err != nil {
		log.WithFields(log.Fields{
			"backendId": backendId,
		}).WithError(err).Error("failed to initialize aws session")
		return nil, err
	}
	client := secretsmanager.NewFromConfig(*cfg)

	// GetSecretValue
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(backendConfig["secretId"]),
	}
	out, err := client.GetSecretValue(context.TODO(), input)
	if err != nil {
		log.WithFields(log.Fields{
			"backendId":   backendId,
			"backendType": backendConfig["type"],
			"secretId":    backendConfig["secretId"],
			"accessKeyId": backendConfig["accessKeyId"],
			"awsProfile":  backendConfig["awsProfile"],
		}).WithError(err).Error("failed to retrieve secret value")
		return nil, err
	}

	secretValue := make(map[string]string, 0)
	if err := json.Unmarshal([]byte(*out.SecretString), &secretValue); err != nil {
		log.WithFields(log.Fields{
			"backendId":   backendId,
			"backendType": backendConfig["type"],
			"secretId":    backendConfig["secretId"],
			"accessKeyID": backendConfig["accessKeyID"],
			"awsProfile":  backendConfig["awsProfile"],
		}).WithError(err).Error("failed to retrieve secret value")
		return nil, err
	}

	secret := &AwsSecretsManagerBackend{
		BackendId: backendId,
		Client:    client,
		Config:    backendConfig,
		Secret:    secretValue,
	}
	return secret, nil
}

func (b *AwsSecretsManagerBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	if val, ok := b.Secret[secretKey]; ok {
		return secret.SecretOutput{Value: &val, Error: nil}
	}
	es := errors.New("backend does not provide secret key").Error()

	log.WithFields(log.Fields{
		"backendId":   b.BackendId,
		"backendType": b.Config["type"],
		"secretName":  b.Config["secretName"],
	}).Error("backend does not provide secret key")
	return secret.SecretOutput{Value: nil, Error: &es}
}
