package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

func GetSecretsManagerValue(arn string) (string, error) {
	svc := secretsmanager.New(session.New())
	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(arn),
	}

	result, err := svc.GetSecretValue(input)
	if err != nil {
		return "", err
	}

	return aws.StringValue(result.SecretString), nil
}
