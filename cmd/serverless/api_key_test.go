// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package main

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/aws/aws-sdk-go/service/kms/kmsiface"
	"github.com/stretchr/testify/assert"
)

// mockEncryptedApiKeyBase64 represents an API key encrypted with KMS and encoded as a base64 string
const mockEncryptedApiKeyBase64 = "MjIyMjIyMjIyMjIyMjIyMg=="

// mockEncryptedApiKey represents the encrypted API key after it has been decoded from base64
const mockEncryptedApiKey = "2222222222222222"

// expectedDecryptedApiKey represents the true value of the API key after decryption by KMS
const expectedDecryptedApiKey = "1111111111111111"

// mockFunctionName represents the name of the current function
var mockFunctionName = "my-Function"

type mockKMSClientWithEncryptionContext struct {
	kmsiface.KMSAPI
}

func (_ mockKMSClientWithEncryptionContext) Decrypt(params *kms.DecryptInput) (*kms.DecryptOutput, error) {
	if *params.EncryptionContext[encryptionContextKey] != mockFunctionName {
		return nil, errors.New("InvalidCiphertextExeption")
	}
	if bytes.Equal(params.CiphertextBlob, []byte(mockEncryptedApiKey)) {
		return &kms.DecryptOutput{
			Plaintext: []byte(expectedDecryptedApiKey),
		}, nil
	}
	return nil, errors.New("KMS error")
}

type mockKMSClientNoEncryptionContext struct {
	kmsiface.KMSAPI
}

func (_ mockKMSClientNoEncryptionContext) Decrypt(params *kms.DecryptInput) (*kms.DecryptOutput, error) {
	if params.EncryptionContext[encryptionContextKey] != nil {
		return nil, errors.New("InvalidCiphertextExeption")
	}
	if bytes.Equal(params.CiphertextBlob, []byte(mockEncryptedApiKey)) {
		return &kms.DecryptOutput{
			Plaintext: []byte(expectedDecryptedApiKey),
		}, nil
	}
	return nil, errors.New("KMS error")
}

func TestDecryptKMSWithEncryptionContext(t *testing.T) {
	os.Setenv(functionNameEnvVar, mockFunctionName)
	defer os.Setenv(functionNameEnvVar, "")

	client := mockKMSClientWithEncryptionContext{}
	result, _ := decryptKMS(client, mockEncryptedApiKeyBase64)
	assert.Equal(t, expectedDecryptedApiKey, result)
}

func TestDecryptKMSNoEncryptionContext(t *testing.T) {
	client := mockKMSClientNoEncryptionContext{}
	result, _ := decryptKMS(client, mockEncryptedApiKeyBase64)
	assert.Equal(t, expectedDecryptedApiKey, result)
}
