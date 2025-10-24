// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package cloudauth provides the implementation for specific delegated auth exchanges
package cloudauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/delegatedauth"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// SigningData is the data structure that represents the Data used to generate and AWS Proof
type SigningData struct {
	HeadersEncoded string `json:"iam_headers_encoded"`
	BodyEncoded    string `json:"iam_body_encoded"`
	URLEncoded     string `json:"iam_url_encoded"`
	Method         string `json:"iam_method"`
}

const (
	// orgIDHeader is the header we use to specify the name of the org we request a token for
	orgIDHeader       = "x-ddog-org-id"
	contentTypeHeader = "Content-Type"
	applicationForm   = "application/x-www-form-urlencoded; charset=utf-8"

	awsAccessKeyIdName     = "AWS_ACCESS_KEY_ID"
	awsSecretAccessKeyName = "AWS_SECRET_ACCESS_KEY"
	awsSessionTokenName    = "AWS_SESSION_TOKEN"

	defaultRegion         = "us-east-1"
	defaultStsHost        = "sts.amazonaws.com"
	regionalStsHost       = "sts.%s.amazonaws.com"
	service               = "sts"
	getCallerIdentityBody = "Action=GetCallerIdentity&Version=2011-06-15"
)

// ProviderAWS is the specifier for the AWS provider type
const ProviderAWS = "aws"

type AWSAuth struct {
	AwsRegion string
}

func (a *AWSAuth) GetApiKey(cfg pkgconfigmodel.Reader, config *delegatedauth.AuthConfig) (*string, error) {
	// Get local AWS Credentials
	creds := a.getCredentials(cfg)

	if config == nil || config.OrgUUID == "" {
		return nil, fmt.Errorf("missing org UUID in config")
	}

	// Use the credentials to generate the signing data
	data, err := a.generateAwsAuthData(config.OrgUUID, creds)
	if err != nil {
		return nil, err
	}

	// Generate the auth string passed to the token endpoint
	authString := data.BodyEncoded + "|" + data.HeadersEncoded + "|" + data.Method + "|" + data.URLEncoded

	authResponse, err := delegatedauth.GetApiKey(cfg, config.OrgUUID, authString)
	return authResponse, err
}

// getCredentials retrieves AWS credentials using the same approach as EC2 tags fetching.
// It first tries config/environment variables, then falls back to EC2 instance metadata service.
func (a *AWSAuth) getCredentials(cfg pkgconfigmodel.Reader) *ec2.SecurityCredentials {
	creds := &ec2.SecurityCredentials{}

	// First, try to get credentials from config
	creds.AccessKeyID = cfg.GetString(awsAccessKeyIdName)
	creds.SecretAccessKey = cfg.GetString(awsSecretAccessKeyName)
	creds.Token = cfg.GetString(awsSessionTokenName)

	// Then try environment variables
	if creds.AccessKeyID == "" {
		creds.AccessKeyID = os.Getenv(awsAccessKeyIdName)
	}
	if creds.SecretAccessKey == "" {
		creds.SecretAccessKey = os.Getenv(awsSecretAccessKeyName)
	}
	if creds.Token == "" {
		creds.Token = os.Getenv(awsSessionTokenName)
	}

	// If we have explicit credentials, return them
	if creds.AccessKeyID != "" && creds.SecretAccessKey != "" {
		return creds
	}

	// Fall back to EC2 instance metadata service (same as ec2_tags.go does)
	log.Debugf("No explicit AWS credentials found in config or environment, trying EC2 instance metadata service")
	ctx := context.Background()
	ec2Creds, err := ec2.GetSecurityCredentials(ctx)
	if err != nil {
		log.Warnf("Failed to get credentials from EC2 instance metadata: %v", err)
		return creds
	}

	log.Infof("Successfully retrieved AWS credentials from EC2 instance metadata service")
	return ec2Creds
}

func (a *AWSAuth) getConnectionParameters() (string, string, string) {
	region := a.AwsRegion
	var host string
	// Default to the default global STS Host (see here: https://docs.aws.amazon.com/general/latest/gr/sts.html)
	if region == "" {
		region = defaultRegion
		host = defaultStsHost
	} else {
		// If the region is not empty, use the regional STS host
		host = fmt.Sprintf(regionalStsHost, region)
	}
	stsFullURL := fmt.Sprintf("https://%s", host)
	return stsFullURL, region, host
}

func (a *AWSAuth) getUserAgent() string {
	return fmt.Sprintf("datadog-agent/%s", version.AgentVersion)
}

func (a *AWSAuth) generateAwsAuthData(orgUUID string, creds *ec2.SecurityCredentials) (*SigningData, error) {
	if orgUUID == "" {
		return nil, fmt.Errorf("missing org UUID")
	}
	if creds == nil || (creds.AccessKeyID == "" && creds.SecretAccessKey == "") || creds.Token == "" {
		return nil, fmt.Errorf("missing AWS credentials")
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
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.Token,
	}

	// Create the v4 signer
	signer := v4.NewSigner()

	// Sign the request
	// The orgIDHeader is already set on the request, so it will be included in the signature
	now := time.Now().UTC()
	err = signer.SignHTTP(context.Background(), awsCreds, req, payloadHash, service, region, now)
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

	return &SigningData{
		HeadersEncoded: base64.StdEncoding.EncodeToString(headersJSON),
		BodyEncoded:    base64.StdEncoding.EncodeToString(bodyBytes),
		Method:         http.MethodPost,
		URLEncoded:     base64.StdEncoding.EncodeToString([]byte(stsFullURL)),
	}, nil
}
