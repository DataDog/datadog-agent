// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// imdsEventData builds the kernel's EVENT_IMDS layout: source u32 + payload
func imdsEventData(source CredentialSource, payload string) []byte {
	data := make([]byte, 4, 4+len(payload))
	binary.NativeEndian.PutUint32(data, uint32(source))
	return append(data, payload...)
}

// EKS Pod Identity /v1/credentials response shape
const podIdentityResponse = "HTTP/1.1 200 OK\r\n" +
	"Content-Type: application/json\r\n" +
	"\r\n" +
	`{"AccessKeyId":"ASIAIOSFODNN7EXAMPLE","SecretAccessKey":"wJalrXUtnFEMI/K7MDENG","Token":"FQoDYXdzEL3EXAMPLETOKEN","AccountId":"123456789012","Expiration":"2324-05-01T12:00:00Z"}`

const imdsCredentialsResponse = "HTTP/1.1 200 OK\r\n" +
	"Content-Type: application/json\r\n" +
	"Server: EC2ws\r\n" +
	"\r\n" +
	`{"Code":"Success","LastUpdated":"2012-04-26T16:39:16Z","Type":"AWS-HMAC","AccessKeyId":"ASIAIOSFODNN7EXAMPLE","SecretAccessKey":"wJalrXUtnFEMI/K7MDENG","Token":"FQoDYXdzEL3EXAMPLETOKEN","Expiration":"2324-05-01T12:00:00Z"}`

func TestIMDSEventUnmarshalCredentialSource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		source   CredentialSource
		expected string
	}{
		{"imds", CredentialSourceIMDS, CredentialSourceIMDSStr},
		{"eks pod identity", CredentialSourceEKSPodIdentity, CredentialSourceEKSPodIdentityStr},
		{"ecs", CredentialSourceECS, CredentialSourceECSStr},
		{"unknown", CredentialSourceUnknown, CredentialSourceUnknownStr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &IMDSEvent{}
			data := imdsEventData(tc.source, podIdentityResponse)

			read, err := e.UnmarshalBinary(data)
			require.NoError(t, err)
			assert.Equal(t, len(data), read, "the whole buffer should be consumed")
			assert.Equal(t, tc.expected, e.CredentialSource)
		})
	}
}

func TestIMDSEventUnmarshalPodIdentityResponse(t *testing.T) {
	e := &IMDSEvent{}
	data := imdsEventData(CredentialSourceEKSPodIdentity, podIdentityResponse)

	_, err := e.UnmarshalBinary(data)
	require.NoError(t, err)

	assert.Equal(t, IMDSResponseType, e.Type)
	assert.Equal(t, CredentialSourceEKSPodIdentityStr, e.CredentialSource)
	assert.Equal(t, IMDSAWSCloudProvider, e.CloudProvider)
	assert.Equal(t, "ASIAIOSFODNN7EXAMPLE", e.AWS.SecurityCredentials.AccessKeyID)
	assert.Equal(t, "2324-05-01T12:00:00Z", e.AWS.SecurityCredentials.ExpirationRaw)
	assert.False(t, e.AWS.SecurityCredentials.Expiration.IsZero(), "expiration should be parsed")
	assert.False(t, e.AWS.IsIMDSv2)
}

func TestIMDSEventUnmarshalIMDSv2NotReportedForPodIdentity(t *testing.T) {
	withV2Header := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"x-aws-ec2-metadata-token-ttl-seconds: 21600\r\n" +
		"\r\n" +
		`{"AccessKeyId":"ASIAIOSFODNN7EXAMPLE"}`

	podIdentity := &IMDSEvent{}
	_, err := podIdentity.UnmarshalBinary(imdsEventData(CredentialSourceEKSPodIdentity, withV2Header))
	require.NoError(t, err)
	assert.False(t, podIdentity.AWS.IsIMDSv2, "is_imds_v2 must not be set for a non-IMDS source")

	// the same payload from the instance metadata service is IMDSv2
	imds := &IMDSEvent{}
	_, err = imds.UnmarshalBinary(imdsEventData(CredentialSourceIMDS, withV2Header))
	require.NoError(t, err)
	assert.True(t, imds.AWS.IsIMDSv2)
}

func TestIMDSEventUnmarshalIMDSResponse(t *testing.T) {
	e := &IMDSEvent{}
	_, err := e.UnmarshalBinary(imdsEventData(CredentialSourceIMDS, imdsCredentialsResponse))
	require.NoError(t, err)

	assert.Equal(t, IMDSResponseType, e.Type)
	assert.Equal(t, CredentialSourceIMDSStr, e.CredentialSource)
	assert.Equal(t, "EC2ws", e.Server)
	assert.Equal(t, "ASIAIOSFODNN7EXAMPLE", e.AWS.SecurityCredentials.AccessKeyID)
	assert.Equal(t, "Success", e.AWS.SecurityCredentials.Code)
	assert.Equal(t, "AWS-HMAC", e.AWS.SecurityCredentials.Type)
}

func TestIMDSEventUnmarshalPodIdentityRequest(t *testing.T) {
	request := "GET /v1/credentials HTTP/1.1\r\n" +
		"Host: 169.254.170.23\r\n" +
		"Authorization: Bearer some-service-account-token\r\n" +
		"User-Agent: aws-sdk-go-v2\r\n" +
		"\r\n"

	e := &IMDSEvent{}
	_, err := e.UnmarshalBinary(imdsEventData(CredentialSourceEKSPodIdentity, request))
	require.NoError(t, err)

	assert.Equal(t, IMDSRequestType, e.Type)
	assert.Equal(t, CredentialSourceEKSPodIdentityStr, e.CredentialSource)
	assert.Equal(t, "/v1/credentials", e.URL)
	assert.Equal(t, "169.254.170.23", e.Host)
	assert.Equal(t, "aws-sdk-go-v2", e.UserAgent)
}

func TestIMDSEventUnmarshalNotEnoughData(t *testing.T) {
	// shorter than the credential source itself
	e := &IMDSEvent{}
	_, err := e.UnmarshalBinary([]byte{0, 0, 0})
	assert.ErrorIs(t, err, ErrNotEnoughData)

	// a credential source but no usable payload
	e = &IMDSEvent{}
	_, err = e.UnmarshalBinary(imdsEventData(CredentialSourceIMDS, "GET /"))
	assert.ErrorIs(t, err, ErrNotEnoughData)
}

func TestIMDSEventUnmarshalNonOKResponse(t *testing.T) {
	e := &IMDSEvent{}
	data := imdsEventData(CredentialSourceEKSPodIdentity, "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n")

	read, err := e.UnmarshalBinary(data)
	assert.ErrorIs(t, err, ErrNoUsefulData)
	assert.Equal(t, len(data), read, "the whole buffer should be reported as consumed")
}
