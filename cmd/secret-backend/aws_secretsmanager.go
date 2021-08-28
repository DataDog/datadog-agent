package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type AwsSecretsManagerSecret struct {
	Client *secretsmanager.Client
	Config map[string]string
	Value  map[string]string
}

func NewAwsSecretsManagerSecret(secretConfig map[string]string, secretId string) (
	*AwsSecretsManagerSecret, error) {

	var cfg aws.Config
	var client *secretsmanager.Client

	if _, ok := secretConfig["accessKeyId"]; ok {
		if _, ok := secretConfig["secretAccessKey"]; !ok {
			log.WithFields(log.Fields{
				"secret": secretId,
			}).Error("missing required configuration parameter: secretAccessKey")
			return nil, fmt.Errorf("missing required configuration parameter: %s", "secretAccessKey")
		}

		cfg, err := config.LoadDefaultConfig(context.TODO(),
			config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
				Value: aws.Credentials{
					AccessKeyID:     secretConfig["accessKeyId"],
					SecretAccessKey: secretConfig["secretAccessKey"],
				},
			}),
			config.WithRegion(secretConfig["awsRegion"]),
		)
		if err != nil {
			log.WithFields(log.Fields{
				"secret":      secretId,
				"secretType":  secretConfig["type"],
				"secretName":  secretConfig["secretName"],
				"accessKeyId": secretConfig["accessKeyId"],
			}).WithError(err).Error("aws static credential failure")
			return nil, err
		}

		client = secretsmanager.NewFromConfig(cfg)
	}

	// GetSecretValue
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretConfig["secretName"]),
	}
	out, err := client.GetSecretValue(context.TODO(), input)
	if err != nil {
		log.WithFields(log.Fields{
			"secret":      secretId,
			"secretName":  secretConfig["secretName"],
			"secretType":  secretConfig["type"],
			"accessKeyId": secretConfig["accessKeyId"],
		}).WithError(err).Error("failed to retrieve secret value")
		return nil, err
	}

	secretValue := make(map[string]string, 0)
	if err := json.Unmarshal([]byte(*out.SecretString), &secretValue); err != nil {
		log.WithFields(log.Fields{
			"secret":      secretId,
			"secretType":  secretConfig["type"],
			"accessKeyID": secretConfig["accessKeyID"],
		}).WithError(err).Error("failed to retrieve secret value")
		return nil, err
	}

	secret := &AwsSecretsManagerSecret{
		Client: secretsmanager.NewFromConfig(cfg),
		Config: secretConfig,
		Value:  secretValue,
	}
	return secret, nil
}

func (s *AwsSecretsManagerSecret) GetSecretOutput(secretKey string) SecretOutput {
	if val, ok := s.Value[secretKey]; ok {
		return SecretOutput{Value: &val, Error: nil}
	}
	errorStr := errors.New("undefined secret value field key").Error()
	return SecretOutput{Value: nil, Error: &errorStr}
}
