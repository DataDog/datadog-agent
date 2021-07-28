// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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

// mockEncryptedAPIKeyBase64 represents an API key encrypted with KMS and encoded as a base64 string
const mockEncryptedAPIKeyBase64 = "MjIyMjIyMjIyMjIyMjIyMg=="

// mockEncryptedAPIKey represents the encrypted API key after it has been decoded from base64
const mockEncryptedAPIKey = "2222222222222222"

// expectedDecryptedAPIKey represents the true value of the API key after decryption by KMS
const expectedDecryptedAPIKey = "1111111111111111"

// mockFunctionName represents the name of the current function
var mockFunctionName = "my-Function"

type mockKMSClientWithEncryptionContext struct {
	kmsiface.KMSAPI
}

func (mockKMSClientWithEncryptionContext) Decrypt(params *kms.DecryptInput) (*kms.DecryptOutput, error) {
	if *params.EncryptionContext[encryptionContextKey] != mockFunctionName {
		return nil, errors.New("InvalidCiphertextExeption")
	}
	if bytes.Equal(params.CiphertextBlob, []byte(mockEncryptedAPIKey)) {
		return &kms.DecryptOutput{
			Plaintext: []byte(expectedDecryptedAPIKey),
		}, nil
	}
	return nil, errors.New("KMS error")
}

type mockKMSClientNoEncryptionContext struct {
	kmsiface.KMSAPI
}

func (mockKMSClientNoEncryptionContext) Decrypt(params *kms.DecryptInput) (*kms.DecryptOutput, error) {
	if params.EncryptionContext[encryptionContextKey] != nil {
		return nil, errors.New("InvalidCiphertextExeption")
	}
	if bytes.Equal(params.CiphertextBlob, []byte(mockEncryptedAPIKey)) {
		return &kms.DecryptOutput{
			Plaintext: []byte(expectedDecryptedAPIKey),
		}, nil
	}
	return nil, errors.New("KMS error")
}

func TestDecryptKMSWithEncryptionContext(t *testing.T) {
	os.Setenv(functionNameEnvVar, mockFunctionName)
	defer os.Setenv(functionNameEnvVar, "")

	client := mockKMSClientWithEncryptionContext{}
	result, _ := decryptKMS(client, mockEncryptedAPIKeyBase64)
	assert.Equal(t, expectedDecryptedAPIKey, result)
}

func TestDecryptKMSNoEncryptionContext(t *testing.T) {
	client := mockKMSClientNoEncryptionContext{}
	result, _ := decryptKMS(client, mockEncryptedAPIKeyBase64)
	assert.Equal(t, expectedDecryptedAPIKey, result)
}
