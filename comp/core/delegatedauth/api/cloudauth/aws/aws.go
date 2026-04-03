// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package aws provides the implementation for aws auth exchange
package aws

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/aws/creds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// signingData is the data structure that represents the Data used to generate an AWS Proof
type signingData struct {
	headersEncoded string
	bodyEncoded    string
	urlEncoded     string
	method         string
}

const (
	// orgIDHeader is the header we use to specify the name of the org we request a token for
	orgIDHeader       = "x-ddog-org-id"
	contentTypeHeader = "Content-Type"
	applicationForm   = "application/x-www-form-urlencoded; charset=utf-8"

	// Environment variable names
	awsAccessKeyIDEnvVar     = "AWS_ACCESS_KEY_ID"
	awsSecretAccessKeyEnvVar = "AWS_SECRET_ACCESS_KEY"
	awsSessionTokenEnvVar    = "AWS_SESSION_TOKEN"

	defaultRegion         = "us-east-1"
	defaultStsHost        = "sts.amazonaws.com"
	regionalStsHost       = "sts.%s.amazonaws.com"
	service               = "sts"
	getCallerIdentityBody = "Action=GetCallerIdentity&Version=2011-06-15"
)

// AWSAuth contains the implementation for the AWS cloud auth
type AWSAuth struct {
	region string
}

// NewAWSAuth creates a new AWSAuth from an AWSProviderConfig.
func NewAWSAuth(config *cloudauthconfig.AWSProviderConfig) *AWSAuth {
	region := ""
	if config != nil {
		region = config.Region
	}
	return &AWSAuth{
		region: region,
	}
}

// GenerateAuthProof generates an AWS-specific authentication proof using SigV4 signing.
// This proof includes a signed AWS STS GetCallerIdentity request that proves access to AWS credentials.
// The context parameter allows for cancellation of the proof generation.
func (a *AWSAuth) GenerateAuthProof(ctx context.Context, _ pkgconfigmodel.Reader, config *common.AuthConfig) (string, error) {
	// Check for context cancellation early
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Get local AWS Credentials
	credentials := a.getCredentials(ctx)

	if config == nil || config.OrgUUID == "" {
		return "", errors.New("missing org UUID in config")
	}

	// Use the credentials to generate the signing data
	data, err := a.generateAwsAuthData(ctx, config.OrgUUID, credentials)
	if err != nil {
		return "", err
	}

	// Generate the auth string that will be passed to the Datadog API
	// Format: "<base64-body>|<base64-headers>|<method>|<base64-url>"
	// - body: Base64-encoded request body (GetCallerIdentity action)
	// - headers: Base64-encoded JSON map of HTTP headers (includes Authorization with SigV4 signature)
	// - method: HTTP method (POST)
	// - url: Base64-encoded STS endpoint URL
	authProof := data.bodyEncoded + "|" + data.headersEncoded + "|" + data.method + "|" + data.urlEncoded
	return authProof, nil
}

// getCredentials retrieves AWS credentials using the same approach as EC2 tags fetching.
// It first tries environment variables, then falls back to EC2 instance metadata service.
//
// Note: This function fetches credentials on every call rather than caching them.
// Since GenerateAuthProof is called at the refresh interval (typically 60 minutes),
// and IMDS credentials are valid for several hours, this is acceptable. Future
// optimization could add credential caching with expiration handling if needed.
func (a *AWSAuth) getCredentials(ctx context.Context) *creds.SecurityCredentials {
	awsCredentials := &creds.SecurityCredentials{}

	// Try to get credentials from environment variables
	awsCredentials.AccessKeyID = os.Getenv(awsAccessKeyIDEnvVar)
	awsCredentials.SecretAccessKey = os.Getenv(awsSecretAccessKeyEnvVar)
	awsCredentials.Token = os.Getenv(awsSessionTokenEnvVar)

	// If we have explicit credentials, return them
	if awsCredentials.AccessKeyID != "" && awsCredentials.SecretAccessKey != "" {
		return awsCredentials
	}

	// Fall back to EC2 instance metadata service (same as ec2_tags.go does)
	log.Debugf("No explicit AWS credentials found in config or environment, trying EC2 instance metadata service")
	ec2Creds, err := creds.GetSecurityCredentials(ctx)
	if err != nil {
		log.Warnf("Failed to get credentials from EC2 instance metadata: %v", err)
		return awsCredentials
	}

	log.Infof("Successfully retrieved AWS credentials from EC2 instance metadata service")
	return ec2Creds
}

func (a *AWSAuth) getConnectionParameters() (string, string, string) {
	region := a.region
	var host string
	// Default to the default global STS Host (see here: https://docs.aws.amazon.com/general/latest/gr/sts.html)
	if region == "" {
		region = defaultRegion
		host = defaultStsHost
	} else {
		// If the region is not empty, use the regional STS host
		host = fmt.Sprintf(regionalStsHost, region)
	}
	stsFullURL := "https://" + host
	return stsFullURL, region, host
}

func (a *AWSAuth) getUserAgent() string {
	return "datadog-agent/" + version.AgentVersion
}

func (a *AWSAuth) generateAwsAuthData(ctx context.Context, orgUUID string, awsCredentials *creds.SecurityCredentials) (*signingData, error) {
	if orgUUID == "" {
		return nil, errors.New("missing org UUID")
	}
	if awsCredentials == nil || awsCredentials.AccessKeyID == "" || awsCredentials.SecretAccessKey == "" {
		return nil, errors.New("missing AWS credentials")
	}
	stsFullURL, region, host := a.getConnectionParameters()

	// Create the request body
	requestBody := getCallerIdentityBody
	bodyBytes := []byte(requestBody)

	// Calculate the payload hash manually
	payloadHashBytes := sha256.Sum256(bodyBytes)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])

	// Create a seekable body reader
	bodyReader := bytes.NewReader(bodyBytes)

	// Create an HTTP request
	req, err := http.NewRequest(http.MethodPost, stsFullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers before signing
	req.Header.Set(contentTypeHeader, applicationForm)
	req.Header.Set(orgIDHeader, orgUUID)
	req.Header.Set("User-Agent", a.getUserAgent())
	req.ContentLength = int64(len(bodyBytes))
	req.Host = host

	// Create AWS credentials from our EC2 credentials
	awsCreds := aws.Credentials{
		AccessKeyID:     awsCredentials.AccessKeyID,
		SecretAccessKey: awsCredentials.SecretAccessKey,
		SessionToken:    awsCredentials.Token,
	}

	// Create the v4 signer
	signer := v4.NewSigner()

	// Sign the request
	// The orgIDHeader is already set on the request, so it will be included in the signature
	now := time.Now().UTC()
	err = signer.SignHTTP(ctx, awsCreds, req, payloadHash, service, region, now)
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	// Extract headers from the signed request
	headerMap := make(map[string][]string)
	for key, values := range req.Header {
		headerMap[key] = values
	}
	headerMap["Host"] = []string{host}

	// Marshal headers to JSON
	headersJSON, err := json.Marshal(headerMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal headers: %w", err)
	}

	return &signingData{
		headersEncoded: base64.StdEncoding.EncodeToString(headersJSON),
		bodyEncoded:    base64.StdEncoding.EncodeToString(bodyBytes),
		method:         http.MethodPost,
		urlEncoded:     base64.StdEncoding.EncodeToString([]byte(stsFullURL)),
	}, nil
}
