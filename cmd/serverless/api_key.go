// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/aws/aws-sdk-go/service/kms/kmsiface"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

// encryptionContextKey is the key added to the encryption context by the Lambda console UI
const encryptionContextKey = "LambdaFunctionName"

// functionNameEnvVar is the environment variable that stores the function name.
const functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"

// decryptKMS decodes and deciphers the base64-encoded ciphertext given as a parameter using KMS.
// For this to work properly, the Lambda function must have the appropriate IAM permissions.
func decryptKMS(kmsClient kmsiface.KMSAPI, ciphertext string) (string, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("Failed to decode ciphertext from base64: %v", err)
	}

	// When the API key is encrypted using the AWS console, the function name is added as an
	// encryption context. When the API key is encrypted using the AWS CLI, no encryption context
	// is added. We need to try decrypting the API key both with and without the encryption context.

	// Try without encryption context, in case API key was encrypted using the AWS CLI
	functionName := os.Getenv(functionNameEnvVar)
	params := &kms.DecryptInput{
		CiphertextBlob: decodedBytes,
	}
	response, err := kmsClient.Decrypt(params)

	if err != nil {
		log.Debug("Failed to decrypt ciphertext without encryption context, retrying with encryption context")
		// Try with encryption context, in case API key was encrypted using the AWS Console
		params = &kms.DecryptInput{
			CiphertextBlob: decodedBytes,
			EncryptionContext: map[string]*string{
				encryptionContextKey: &functionName,
			},
		}
		response, err = kmsClient.Decrypt(params)
		if err != nil {
			return "", fmt.Errorf("Failed to decrypt ciphertext with kms: %v", err)
		}
	}

	plaintext := string(response.Plaintext)
	return plaintext, nil
}

// readAPIKeyFromKMS gets and decrypts an API key encrypted with KMS if the env var DD_KMS_API_KEY has been set.
// If none has been set, it returns an empty string and a nil error.
func readAPIKeyFromKMS() (string, error) {
	cipherText := os.Getenv(kmsAPIKeyEnvVar)
	if cipherText == "" {
		return "", nil
	}
	log.Debugf("Found %s, trying to decipher it.", kmsAPIKeyEnvVar)
	kmsClient := kms.New(session.New(nil))
	plaintext, err := decryptKMS(kmsClient, cipherText)
	if err != nil {
		return "", fmt.Errorf("decryptKMS error: %s", err)
	}
	return plaintext, nil
}

// readAPIKeyFromSecretsManager reads an API Key from AWS Secrets Manager if the env var DD_API_KEY_SECRET_ARN has been set.
// If none has been set, it returns an empty string and a nil error.
func readAPIKeyFromSecretsManager() (string, error) {
	arn := os.Getenv(secretsManagerAPIKeyEnvVar)
	if arn == "" {
		return "", nil
	}
	log.Debugf("Found %s value, trying to use it.", secretsManagerAPIKeyEnvVar)
	secretsManagerClient := secretsmanager.New(session.New(nil))
	secret := &secretsmanager.GetSecretValueInput{}
	secret.SetSecretId(arn)

	output, err := secretsManagerClient.GetSecretValue(secret)
	if err != nil {
		return "", fmt.Errorf("Secrets Manager read error: %s", err)
	}

	if output.SecretString != nil {
		secretString := *output.SecretString // create a copy to not return an object within the AWS response
		return secretString, nil
	} else if output.SecretBinary != nil {
		decodedBinarySecretBytes := make([]byte, base64.StdEncoding.DecodedLen(len(output.SecretBinary)))
		len, err := base64.StdEncoding.Decode(decodedBinarySecretBytes, output.SecretBinary)
		if err != nil {
			return "", fmt.Errorf("Can't base64 decode Secrets Manager secret: %s", err)
		}
		return string(decodedBinarySecretBytes[:len]), nil
	}
	// should not happen but let's handle this gracefully
	log.Warn("Secrets Manager returned something but there seems to be no data available")
	return "", nil
}
