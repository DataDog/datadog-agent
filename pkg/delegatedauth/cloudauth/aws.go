// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package cloudauth provides the implementation for specific delegated auth exchanges
package cloudauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

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
	orgIDHeader         = "x-ddog-org-id"
	hostHeader          = "host"
	contextLengthHeader = "Content-Length"
	contentTypeHeader   = "Content-Type"
	applicationForm     = "application/x-www-form-urlencoded; charset=utf-8"

	awsAccessKeyIdName     = "AWS_ACCESS_KEY_ID"
	awsSecretAccessKeyName = "AWS_SECRET_ACCESS_KEY"
	awsSessionTokenName    = "AWS_SESSION_TOKEN"

	amzDateHeader         = "X-Amz-Date"
	amzTokenHeader        = "X-Amz-Security-Token"
	amzDateFormat         = "20060102"
	amzDateTimeFormat     = "20060102T150405Z"
	defaultRegion         = "us-east-1"
	defaultStsHost        = "sts.amazonaws.com"
	regionalStsHost       = "sts.%s.amazonaws.com"
	service               = "sts"
	algorithm             = "AWS4-HMAC-SHA256"
	aws4Request           = "aws4_request"
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

	now := time.Now().UTC()

	requestBody := getCallerIdentityBody
	h := sha256.Sum256([]byte(requestBody))
	payloadHash := hex.EncodeToString(h[:])

	// Create the headers that factor into the signing algorithm
	headerMap := map[string][]string{
		contextLengthHeader: {
			fmt.Sprintf("%d", len(requestBody)),
		},
		contentTypeHeader: {
			applicationForm,
		},
		amzDateHeader: {
			now.Format(amzDateTimeFormat),
		},
		orgIDHeader: {
			orgUUID,
		},
		amzTokenHeader: {
			creds.Token,
		},
		hostHeader: {
			host,
		},
	}

	headerArr := make([]string, len(headerMap), len(headerMap))
	signedHeadersArr := make([]string, len(headerMap), len(headerMap))
	headerIdx := 0
	for k, v := range headerMap {
		loweredHeaderName := strings.ToLower(k)
		headerArr[headerIdx] = fmt.Sprintf("%s:%s", loweredHeaderName, strings.Join(v, ","))
		signedHeadersArr[headerIdx] = loweredHeaderName
		headerIdx++
	}
	sort.Strings(headerArr)
	sort.Strings(signedHeadersArr)
	signedHeaders := strings.Join(signedHeadersArr, ";")

	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		"", // No query string
		strings.Join(headerArr, "\n") + "\n",
		signedHeaders,
		payloadHash,
	}, "\n")

	// Create the string to sign
	hashCanonicalRequest := sha256.Sum256([]byte(canonicalRequest))
	credentialScope := strings.Join([]string{
		now.Format(amzDateFormat),
		region,
		service,
		aws4Request,
	}, "/")
	stringToSign := a.makeSignature(
		now,
		credentialScope,
		hex.EncodeToString(hashCanonicalRequest[:]),
		region,
		service,
		creds.SecretAccessKey,
		algorithm,
	)

	// Create the authorization header
	credential := strings.Join([]string{
		creds.AccessKeyID,
		credentialScope,
	}, "/")
	authHeader := fmt.Sprintf("%s Credential=%s, SignedHeaders=%s, Signature=%s",
		algorithm, credential, signedHeaders, stringToSign)

	headerMap["Authorization"] = []string{authHeader}
	headerMap["User-Agent"] = []string{a.getUserAgent()}
	headersJSON, err := json.Marshal(headerMap)
	if err != nil {
		return nil, err
	}

	return &SigningData{
		HeadersEncoded: base64.StdEncoding.EncodeToString(headersJSON),
		BodyEncoded:    base64.StdEncoding.EncodeToString([]byte(requestBody)),
		Method:         http.MethodPost,
		URLEncoded:     base64.StdEncoding.EncodeToString([]byte(stsFullURL)),
	}, nil
}

func (a *AWSAuth) makeSignature(t time.Time, credentialScope, payloadHash, region, service, secretAccessKey, algorithm string) string {
	// Create the string to sign
	stringToSign := strings.Join([]string{
		algorithm,
		t.Format(amzDateTimeFormat),
		credentialScope,
		payloadHash,
	}, "\n")

	// Create the signing key
	kDate := hmac256(t.Format(amzDateFormat), []byte("AWS4"+secretAccessKey))
	kRegion := hmac256(region, kDate)
	kService := hmac256(service, kRegion)
	kSigning := hmac256(aws4Request, kService)

	// Sign the string
	signature := hex.EncodeToString(hmac256(stringToSign, kSigning))

	return signature
}

func hmac256(data string, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}
