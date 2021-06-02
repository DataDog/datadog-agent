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
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

// decryptKMS deciphers the ciphertext given as parameter.
func decryptKMS(ciphertext string) (string, error) {
	kmsClient := kms.New(session.New(nil))
	decodedBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("Failed to encode cipher text to base64: %v", err)
	}

	params := &kms.DecryptInput{
		CiphertextBlob: decodedBytes,
	}

	response, err := kmsClient.Decrypt(params)
	if err != nil {
		return "", fmt.Errorf("Failed to decrypt ciphertext with kms: %v", err)
	}
	// Plaintext is a byte array, so convert to string
	decrypted := string(response.Plaintext[:])

	return decrypted, nil
}

// readAPIKeyFromKMS reads an API Key in KMS if the env var DD_KMS_API_KEY has
// been set.
// If none has been set, it returns an empty string and a nil error.
func readAPIKeyFromKMS() (string, error) {
	cipherText := os.Getenv(kmsAPIKeyEnvVar)
	if cipherText == "" {
		return "", nil
	}
	log.Debug("Found DD_KMS_API_KEY value, trying to decipher it.")
	plaintext, err := decryptKMS(cipherText)
	if err != nil {
		return "", fmt.Errorf("decryptKMS error: %s", err)
	}
	return plaintext, nil
}

// readAPIKeyFromSecretsManager reads an API Key from Secrets Manager if the env var DD_API_KEY_SECRET_ARN
// has been set.
// If none has been set, it is returns an empty string and a nil error.
func readAPIKeyFromSecretsManager() (string, error) {
	arn := os.Getenv(secretsManagerAPIKeyEnvVar)
	if arn == "" {
		return "", nil
	}
	log.Debug("Found DD_API_KEY_SECRET_ARN value, trying to use it.")
	ssmClient := secretsmanager.New(session.New(nil))
	secret := &secretsmanager.GetSecretValueInput{}
	secret.SetSecretId(arn)

	output, err := ssmClient.GetSecretValue(secret)
	if err != nil {
		return "", fmt.Errorf("SSM read error: %s", err)
	}

	if output.SecretString != nil {
		secretString := *output.SecretString // create a copy to not return an object within the AWS response
		return secretString, nil
	} else if output.SecretBinary != nil {
		decodedBinarySecretBytes := make([]byte, base64.StdEncoding.DecodedLen(len(output.SecretBinary)))
		len, err := base64.StdEncoding.Decode(decodedBinarySecretBytes, output.SecretBinary)
		if err != nil {
			return "", fmt.Errorf("Can't base64 decode SSM secret: %s", err)
		}
		return string(decodedBinarySecretBytes[:len]), nil
	}
	// should not happen but let's handle this gracefully
	log.Warn("SSM returned something but there seems to be no data available;")
	return "", nil
}
