// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apikey

import (
	"context"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// encryptionContextKey is the key added to the encryption context by the Lambda console UI
const encryptionContextKey = "LambdaFunctionName"

// functionNameEnvVar is the environment variable that stores the function name.
const functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"

// one of those env variable must be set
const apiKeyEnvVar = "DD_API_KEY"
const apiKeySecretManagerEnvVar = "DD_API_KEY_SECRET_ARN"
const apiKeyKmsEnvVar = "DD_KMS_API_KEY"
const apiKeyKmsEncryptedEnvVar = "DD_API_KEY_KMS_ENCRYPTED"

// kmsKeySuffix is the suffix of all environment variables which should be decrypted by KMS
const kmsKeySuffix = "_KMS_ENCRYPTED"

// secretArnSuffix is the suffix of all environment variables which should be decrypted by secrets manager
const secretArnSuffix = "_SECRET_ARN"

type kmsAPI interface {
	Decrypt(context.Context, *kms.DecryptInput, ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// decryptKMS decodes and deciphers the base64-encoded ciphertext given as a parameter using KMS.
// For this to work properly, the Lambda function must have the appropriate IAM permissions.
func decryptKMS(kmsClient kmsAPI, ciphertext string) (string, error) {
	panic("not called")
}

// readAPIKeyFromKMS gets and decrypts an API key encrypted with KMS if the env var DD_KMS_API_KEY has been set.
// If none has been set, it returns an empty string and a nil error.
func readAPIKeyFromKMS(cipherText string) (string, error) {
	panic("not called")
}

// readAPIKeyFromSecretsManager reads an API Key from AWS Secrets Manager if the env var DD_API_KEY_SECRET_ARN has been set.
// If none has been set, it returns an empty string and a nil error.
func readAPIKeyFromSecretsManager(arn string) (string, error) {
	panic("not called")
}

func extractRegionFromSecretsManagerArn(secretsManagerArn string) (string, error) {
	panic("not called")
}

// checkForSingleAPIKey checks if an API key has been set in multiple places and logs a warning if so.
func checkForSingleAPIKey() {
	var apikeySetIn = []string{}
	if len(os.Getenv(apiKeyKmsEncryptedEnvVar)) > 0 {
		apikeySetIn = append(apikeySetIn, "KMS_ENCRYPTED")
	}
	if len(os.Getenv(apiKeyKmsEnvVar)) > 0 {
		apikeySetIn = append(apikeySetIn, "KMS")
	}
	if len(os.Getenv(apiKeySecretManagerEnvVar)) > 0 {
		apikeySetIn = append(apikeySetIn, "SSM")
	}
	if len(os.Getenv(apiKeyEnvVar)) > 0 {
		apikeySetIn = append(apikeySetIn, "environment variable")
	}

	if len(apikeySetIn) > 1 {
		log.Warn("An API Key has been set in multiple places:", strings.Join(apikeySetIn, ", "))
	}
}
